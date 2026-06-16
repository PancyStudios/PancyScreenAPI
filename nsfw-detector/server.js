const util = require('util');
util.isNullOrUndefined = function(obj) { return obj === null || obj === undefined; };

const express = require('express');
const tf = require('@tensorflow/tfjs-node'); // Usar tfjs-node para mejor rendimiento y soporte de filesystem
const nsfw = require('nsfwjs');
const toxicity = require('@tensorflow-models/toxicity');
const { PNG } = require('pngjs');
const path = require('path');

const app = express();
const port = 3001;

let model;
let toxicityModel;
let scamModel;
const toxicityThreshold = 0.85;

async function loadModels() {
  console.log("Loading AI models (NSFW, Toxicity, Scam)...");
  model = await nsfw.load('InceptionV3', { size: 299 });
  console.log("NSFW model loaded successfully.");

  toxicityModel = await toxicity.load(toxicityThreshold);
  console.log("Toxicity model loaded successfully.");

  try {
    const modelPath = path.join(__dirname, 'scam_model', 'model.json');
    scamModel = await tf.loadLayersModel(`file://${modelPath}`);
    console.log("Anti-Scam model loaded successfully.");
  } catch(e) {
    console.error("Anti-Scam model could not be loaded:", e.message);
  }
}

// Raw parser solo para la ruta antigua de imágenes
app.post('/check', express.raw({ type: 'application/octet-stream', limit: '20mb' }), async (req, res) => {
  if (!model) return res.status(503).json({ error: "Model not loaded yet." });
  if (!req.body || req.body.length === 0) return res.status(400).json({ error: "No image data provided." });

  let imageTensor;
  try {
    // Parse PNG image
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
    console.error("Error processing image:", error);
    res.status(500).json({ error: "Failed to process image: " + error.message });
  } finally {
    if (imageTensor) imageTensor.dispose();
  }
});

app.post('/analyze/toxicity', express.json(), async (req, res) => {
  if (!toxicityModel) return res.status(503).json({ error: "Toxicity model not loaded yet." });
  const text = req.body.text;
  if (!text) return res.status(400).json({ error: "No text provided." });

  try {
    const predictions = await toxicityModel.classify([text]);
    // Simplificar la respuesta para ser más amigable
    const result = {};
    let isToxic = false;

    predictions.forEach(p => {
      const isMatch = p.results[0].match;
      result[p.label] = isMatch;
      if (isMatch) isToxic = true;
    });

    res.json({ isToxic, categories: result, raw_predictions: predictions });
  } catch (error) {
    console.error("Error processing text:", error);
    res.status(500).json({ error: "Failed to process text: " + error.message });
  }
});

app.post('/analyze/scam', express.json(), async (req, res) => {
  if (!scamModel) return res.status(503).json({ error: "Scam model not loaded yet." });
  const url = req.body.url;
  if (!url) return res.status(400).json({ error: "No URL provided." });

  try {
    const MAX_LENGTH = 100;
    const chars = new Array(MAX_LENGTH).fill(0);
    for (let i = 0; i < Math.min(url.length, MAX_LENGTH); i++) {
      chars[i] = url.charCodeAt(i) / 255.0;
    }
    
    const tensor = tf.tensor2d([chars]);
    const prediction = scamModel.predict(tensor);
    const score = prediction.dataSync()[0];
    
    res.json({ 
      url: url,
      isScam: score > 0.5,
      scamProbability: score
    });

  } catch (error) {
    console.error("Error processing scam url:", error);
    res.status(500).json({ error: "Failed to analyze URL: " + error.message });
  }
});

app.listen(port, () => console.log(`AI microservice listening at http://localhost:${port}`));
loadModels().catch(console.error);
