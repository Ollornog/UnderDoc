// SPDX-License-Identifier: MIT

// The text of an image, read in the browser of the uploader (ADR-24, the way of the browser): Tesseract.js with German
// and English, LSTM only. Every path is named, so nothing is fetched from a third party. The worker is a file of its own
// (no blob URL), served with the one policy that allows it to compile WebAssembly. Without WebAssembly SIMD the core does
// not load and the text is left to the server.
import Tesseract from './tesseract.esm.min.js';

const base = new URL('.', import.meta.url).href;

/** text reads the words of the image at url and returns them, one line per line of the image. */
export async function text(url) {
  const worker = await Tesseract.createWorker(['deu', 'eng'], Tesseract.OEM.LSTM_ONLY, {
    workerPath: `${base}worker.min.js`,
    corePath: `${base}tesseract-core-simd-lstm.js`,
    langPath: `${base}lang`,
    workerBlobURL: false,
    gzip: true,
    logger: () => {},
    errorHandler: () => {},
  });
  try {
    const { data } = await worker.recognize(url);
    return data.text;
  } finally {
    await worker.terminate();
  }
}
