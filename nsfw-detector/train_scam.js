const util = require('util');
util.isNullOrUndefined = function(obj) { return obj === null || obj === undefined; };

const tf = require('@tensorflow/tfjs-node');
const fs = require('fs');
const path = require('path');

// 1. Cargar el dataset
const data = JSON.parse(fs.readFileSync(path.join(__dirname, 'scam_dataset.json'), 'utf8'));

// 2. Preprocesamiento simple (Bag of Characters)
const MAX_LENGTH = 100;

function urlToTensor(url) {
  const chars = new Array(MAX_LENGTH).fill(0);
  for (let i = 0; i < Math.min(url.length, MAX_LENGTH); i++) {
    chars[i] = url.charCodeAt(i) / 255.0; // Normalización básica
  }
  return chars;
}

const X = [];
const y = [];

data.forEach(item => {
  X.push(urlToTensor(item.url));
  y.push(item.label);
});

const xs = tf.tensor2d(X);
const ys = tf.tensor2d(y, [y.length, 1]);

// 3. Crear el Modelo Neuronal Secuencial
const model = tf.sequential();

model.add(tf.layers.dense({
  units: 64,
  activation: 'relu',
  inputShape: [MAX_LENGTH]
}));

model.add(tf.layers.dense({
  units: 32,
  activation: 'relu'
}));

model.add(tf.layers.dense({
  units: 1,
  activation: 'sigmoid'
}));

// 4. Compilar el Modelo
model.compile({
  optimizer: tf.train.adam(0.01),
  loss: 'binaryCrossentropy',
  metrics: ['accuracy']
});

// 5. Entrenar el Modelo
async function trainAndSave() {
  console.log("Iniciando entrenamiento del modelo Anti-Scam...");
  
  await model.fit(xs, ys, {
    epochs: 50,
    batchSize: 4,
    callbacks: {
      onEpochEnd: (epoch, logs) => {
        console.log(`Epoch ${epoch + 1}: Loss = ${logs.loss.toFixed(4)}, Accuracy = ${(logs.acc * 100).toFixed(2)}%`);
      }
    }
  });

  console.log("Entrenamiento completado.");

  // Guardar el modelo en el sistema de archivos
  const modelPath = path.join(__dirname, 'scam_model');
  await model.save(`file://${modelPath}`);
  console.log(`Modelo guardado exitosamente en ${modelPath}`);
}

trainAndSave();
