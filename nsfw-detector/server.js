const util = require('util');
util.isNullOrUndefined = function(obj) { return obj === null || obj === undefined; };

const express = require('express');
const tf = require('@tensorflow/tfjs-node');
const nsfw = require('nsfwjs');
const toxicity = require('@tensorflow-models/toxicity');
const { PNG } = require('pngjs');
const { createScheduler, createWorker } = require('tesseract.js');
const fs = require('fs');
const path = require('path');

const app = express();
const port = 3001;

// ── Model references ──────────────────────────────────────────────────────────
let model;             // NSFW (InceptionV3)
let toxicityModel;     // Google toxicity
let scamModel;         // Custom anti-scam
let ocrScheduler;      // Tesseract.js scheduler (2 workers for concurrency)

const toxicityThreshold = 0.85;
const MAX_LENGTH = 100;

// ── Model loading ─────────────────────────────────────────────────────────────
async function loadModels() {
  console.log('[AI] Cargando modelos (NSFW, Toxicity, Scam, OCR)...');

  model = await nsfw.load('InceptionV3', { size: 299 });
  console.log('[AI] Modelo NSFW cargado.');

  toxicityModel = await toxicity.load(toxicityThreshold);
  console.log('[AI] Modelo Toxicity cargado.');

  try {
    const modelPath = path.join(__dirname, 'scam_model', 'model.json');
    scamModel = await tf.loadLayersModel(`file://${modelPath}`);
    console.log('[AI] Modelo Anti-Scam cargado.');
  } catch (e) {
    console.error('[AI] No se pudo cargar el modelo Anti-Scam:', e.message);
  }

  // Initialize Tesseract.js OCR scheduler with 2 workers for concurrency
  try {
    ocrScheduler = createScheduler();
    const w1 = await createWorker('eng+spa');
    const w2 = await createWorker('eng+spa');
    ocrScheduler.addWorker(w1);
    ocrScheduler.addWorker(w2);
    console.log('[AI] Scheduler OCR (Tesseract) inicializado con 2 workers.');
  } catch (e) {
    console.error('[AI] Error inicializando OCR:', e.message);
  }
}

// ── NSFW Image Check ──────────────────────────────────────────────────────────
app.post('/check', express.raw({ type: 'application/octet-stream', limit: '20mb' }), async (req, res) => {
  if (!model) return res.status(503).json({ error: 'Model not loaded yet.' });
  if (!req.body || req.body.length === 0) return res.status(400).json({ error: 'No image data provided.' });

  let imageTensor;
  try {
    const png = PNG.sync.read(req.body);
    const numChannels = 3;
    const numPixels = png.width * png.height;
    const values = new Int32Array(numPixels * numChannels);

    for (let i = 0; i < numPixels; i++) {
      for (let channel = 0; channel < numChannels; ++channel) {
        values[i * numChannels + channel] = png.data[i * 4 + channel];
      }
    }

    imageTensor = tf.tensor3d(values, [png.height, png.width, numChannels], 'int32');
    const predictions = await model.classify(imageTensor);

    let isNSFW = false;
    let probability = 0;

    for (const p of predictions) {
      if ((p.className === 'Porn' || p.className === 'Hentai') && p.probability > 0.50) {
        isNSFW = true;
        probability = Math.max(probability, p.probability);
      }
    }

    res.json({ isNSFW, probability, predictions });
  } catch (error) {
    console.error('[NSFW] Error procesando imagen:', error);
    res.status(500).json({ error: 'Failed to process image: ' + error.message });
  } finally {
    if (imageTensor) imageTensor.dispose();
  }
});

// ── Toxicity Analysis ─────────────────────────────────────────────────────────
app.post('/analyze/toxicity', express.json(), async (req, res) => {
  if (!toxicityModel) return res.status(503).json({ error: 'Toxicity model not loaded yet.' });
  const text = req.body.text;
  if (!text) return res.status(400).json({ error: 'No text provided.' });

  try {
    const predictions = await toxicityModel.classify([text]);
    const result = {};
    let isToxic = false;

    predictions.forEach(p => {
      const isMatch = p.results[0].match;
      result[p.label] = isMatch;
      if (isMatch) isToxic = true;
    });

    res.json({ isToxic, categories: result, raw_predictions: predictions });
  } catch (error) {
    console.error('[Toxicity] Error:', error);
    res.status(500).json({ error: 'Failed to process text: ' + error.message });
  }
});

// ── Scam URL Analysis ─────────────────────────────────────────────────────────
app.post('/analyze/scam', express.json(), async (req, res) => {
  if (!scamModel) return res.status(503).json({ error: 'Scam model not loaded yet.' });
  const url = req.body.url;
  if (!url) return res.status(400).json({ error: 'No URL provided.' });

  try {
    const chars = new Array(MAX_LENGTH).fill(0);
    for (let i = 0; i < Math.min(url.length, MAX_LENGTH); i++) {
      chars[i] = url.charCodeAt(i) / 255.0;
    }

    const tensor = tf.tensor2d([chars]);
    const prediction = scamModel.predict(tensor);
    const score = prediction.dataSync()[0];
    tensor.dispose();

    res.json({ url, isScam: score > 0.5, scamProbability: score });
  } catch (error) {
    console.error('[Scam] Error:', error);
    res.status(500).json({ error: 'Failed to analyze URL: ' + error.message });
  }
});

// ── OCR: Text Extraction ──────────────────────────────────────────────────────
app.post('/analyze/ocr', express.raw({ type: 'application/octet-stream', limit: '20mb' }), async (req, res) => {
  if (!ocrScheduler) return res.status(503).json({ error: 'OCR scheduler not initialized.' });
  if (!req.body || req.body.length === 0) return res.status(400).json({ error: 'No image data provided.' });

  try {
    const { data: { text, confidence } } = await ocrScheduler.addJob('recognize', req.body);
    res.json({
      text: text.trim(),
      confidence: parseFloat(confidence.toFixed(2)),
      language: 'eng+spa'
    });
  } catch (error) {
    console.error('[OCR] Error:', error);
    res.status(500).json({ error: 'OCR failed: ' + error.message });
  }
});

// ── Spam Detection (heuristic) ────────────────────────────────────────────────
app.post('/analyze/spam', express.json(), async (req, res) => {
  const text = req.body.text;
  if (!text) return res.status(400).json({ error: 'No text provided.' });

  try {
    const result = analyzeSpam(text);
    res.json(result);
  } catch (error) {
    console.error('[Spam] Error:', error);
    res.status(500).json({ error: 'Spam analysis failed: ' + error.message });
  }
});

function analyzeSpam(text) {
  const categories = {};
  let spamScore = 0;

  // 1. Excessive URLs (more than 2)
  const urlMatches = text.match(/https?:\/\/[^\s]+/g) || [];
  if (urlMatches.length > 2) {
    categories.excessive_urls = urlMatches.length;
    spamScore += 0.25;
  }

  // 2. Discord invite links
  if (/discord\.(gg|com\/invite)/i.test(text)) {
    categories.discord_invite = true;
    spamScore += 0.20;
  }

  // 3. Excessive caps ratio (>50% of letters, at least 10 letters total)
  const letters = text.replace(/[^a-zA-Z]/g, '');
  if (letters.length >= 10) {
    const capsRatio = (text.match(/[A-Z]/g) || []).length / letters.length;
    if (capsRatio > 0.5) {
      categories.excessive_caps = parseFloat(capsRatio.toFixed(2));
      spamScore += 0.20;
    }
  }

  // 4. Repeated characters (aaaa, !!!!)
  if (/(.)\1{4,}/i.test(text)) {
    categories.repeated_chars = true;
    spamScore += 0.15;
  }

  // 5. Common spam / phishing phrases
  const spamPhrases = [
    'free nitro', 'nitro gratis', 'nitro gratuito',
    'click here', 'haga clic', 'haz clic',
    'you won', 'ganaste', 'you have been selected',
    'claim your', 'reclama tu', 'reclama gratis',
    'congratulations', 'felicitaciones',
    '@everyone', '@here',
    'steam gift', 'regalo steam',
    'act now', 'actúa ahora', 'limited time', 'tiempo limitado'
  ];
  const lowerText = text.toLowerCase();
  for (const phrase of spamPhrases) {
    if (lowerText.includes(phrase)) {
      categories.spam_phrase = phrase;
      spamScore += 0.30;
      break;
    }
  }

  // 6. Excessive Discord mentions (>5)
  const mentionCount = (text.match(/<@[!&]?\d+>/g) || []).length;
  if (mentionCount > 5) {
    categories.excessive_mentions = mentionCount;
    spamScore += 0.20;
  }

  // 7. Suspicious TLDs
  if (/https?:\/\/[^\s]+(\.xyz|\.tk|\.pw|\.gq|\.cf|\.ml)\b/i.test(text)) {
    categories.suspicious_tld = true;
    spamScore += 0.25;
  }

  spamScore = Math.min(spamScore, 1.0);

  return {
    isSpam: spamScore > 0.5,
    spamProbability: parseFloat(spamScore.toFixed(4)),
    categories
  };
}

// ── Scam Model Retraining ─────────────────────────────────────────────────────
// Body: { "examples": [{ "url": "...", "label": 0 | 1 }] }
app.post('/ai/scam/train', express.json(), async (req, res) => {
  const { examples } = req.body;

  if (!Array.isArray(examples) || examples.length === 0) {
    return res.status(400).json({ error: 'Se requiere un array "examples" con al menos un ejemplo.' });
  }

  for (const ex of examples) {
    if (typeof ex.url !== 'string' || (ex.label !== 0 && ex.label !== 1)) {
      return res.status(400).json({ error: 'Cada ejemplo debe tener "url" (string) y "label" (0 o 1).' });
    }
  }

  try {
    // Merge with existing dataset
    const datasetPath = path.join(__dirname, 'scam_dataset.json');
    let dataset = [];
    try {
      dataset = JSON.parse(fs.readFileSync(datasetPath, 'utf8'));
    } catch (e) { /* first run */ }

    dataset.push(...examples);
    fs.writeFileSync(datasetPath, JSON.stringify(dataset, null, 2));

    const totalExamples = dataset.length;

    // Respond immediately; retraining happens async
    res.json({
      message: 'Reentrenamiento iniciado en segundo plano.',
      added_examples: examples.length,
      total_examples: totalExamples
    });

    // Async retraining
    retrainScamModel(dataset).then(() => {
      console.log('[Scam] Modelo reentrenado exitosamente.');
    }).catch(err => {
      console.error('[Scam] Error reentrenando modelo:', err.message);
    });

  } catch (error) {
    res.status(500).json({ error: 'Error iniciando reentrenamiento: ' + error.message });
  }
});

async function retrainScamModel(dataset) {
  const X = dataset.map(d => {
    const chars = new Array(MAX_LENGTH).fill(0);
    const url = d.url || '';
    for (let i = 0; i < Math.min(url.length, MAX_LENGTH); i++) {
      chars[i] = url.charCodeAt(i) / 255.0;
    }
    return chars;
  });

  const y = dataset.map(d => d.label);

  const xs = tf.tensor2d(X);
  const ys = tf.tensor2d(y, [y.length, 1]);

  const newModel = tf.sequential();
  newModel.add(tf.layers.dense({ inputShape: [MAX_LENGTH], units: 64, activation: 'relu' }));
  newModel.add(tf.layers.dense({ units: 32, activation: 'relu' }));
  newModel.add(tf.layers.dense({ units: 1, activation: 'sigmoid' }));
  newModel.compile({ optimizer: tf.train.adam(0.01), loss: 'binaryCrossentropy', metrics: ['accuracy'] });

  await newModel.fit(xs, ys, {
    epochs: 50,
    batchSize: 4,
    verbose: 0
  });

  const modelPath = path.join(__dirname, 'scam_model');
  await newModel.save(`file://${modelPath}`);

  // Hot-swap the in-memory model
  scamModel = newModel;

  xs.dispose();
  ys.dispose();
}

// ── Start ─────────────────────────────────────────────────────────────────────
app.listen(port, () => console.log(`[AI] Microservicio escuchando en http://localhost:${port}`));
loadModels().catch(console.error);
