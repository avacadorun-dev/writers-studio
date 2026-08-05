interface CryptoSource {
  randomUUID?: () => string;
  getRandomValues?: (values: Uint8Array) => Uint8Array;
}

export function createClientId(source: CryptoSource | undefined = globalThis.crypto): string {
  if (typeof source?.randomUUID === "function") {
    return source.randomUUID();
  }
  const bytes = new Uint8Array(16);
  if (typeof source?.getRandomValues === "function") {
    source.getRandomValues(bytes);
  } else {
    for (let index = 0; index < bytes.length; index += 1) {
      bytes[index] = Math.floor(Math.random() * 256);
    }
  }
  bytes[6] = (bytes[6] & 0x0f) | 0x40;
  bytes[8] = (bytes[8] & 0x3f) | 0x80;
  const encoded = Array.from(bytes, (value) => value.toString(16).padStart(2, "0")).join("");
  return [
    encoded.slice(0, 8),
    encoded.slice(8, 12),
    encoded.slice(12, 16),
    encoded.slice(16, 20),
    encoded.slice(20),
  ].join("-");
}
