// Browser implementation of RFC 7801. The original C++ implementation and its
// independent known-answer test remain in services/Encryption_Service.
const PI = Uint8Array.from([
    252,238,221,17,207,110,49,22,251,196,250,218,35,197,4,77,233,119,240,219,147,46,153,186,23,54,241,187,20,205,95,193,
    249,24,101,90,226,92,239,33,129,28,60,66,139,1,142,79,5,132,2,174,227,106,143,160,6,11,237,152,127,212,211,31,
    235,52,44,81,234,200,72,171,242,42,104,162,253,58,206,204,181,112,14,86,8,12,118,18,191,114,19,71,156,183,93,135,
    21,161,150,41,16,123,154,199,243,145,120,111,157,158,178,177,50,117,25,61,255,53,138,126,109,84,198,128,195,189,13,
    87,223,245,36,169,62,168,67,201,215,121,214,246,124,34,185,3,224,15,236,222,122,148,176,188,220,232,40,80,78,51,10,
    74,167,151,96,115,30,0,98,68,26,184,56,130,100,159,38,65,173,69,70,146,39,94,85,47,140,163,165,125,105,213,149,59,
    7,88,179,64,134,172,29,247,48,55,107,228,136,217,231,137,225,27,131,73,76,63,248,254,141,83,170,144,202,216,133,97,
    32,113,103,164,45,43,9,91,203,155,37,208,190,229,108,82,89,166,116,210,230,244,180,192,209,102,175,194,57,75,99,182,
]);
const L = [148,32,133,16,194,192,1,251,1,192,194,16,133,32,148,1];
function multiply(a, b) {
    let result = 0;
    for (let bit = 0; bit < 8; bit++) {
        if (b & 1) result ^= a;
        a = ((a << 1) ^ ((a & 128) ? 0xc3 : 0)) & 255;
        b >>>= 1;
    }
    return result;
}
const products = L.map(coefficient => Uint8Array.from({ length: 256 }, (_, value) => multiply(coefficient, value)));
function linear(block) {
    for (let step = 0; step < 16; step++) {
        let first = 0;
        for (let i = 0; i < 16; i++) first ^= products[i][block[i]];
        block.copyWithin(1, 0, 15);
        block[0] = first;
    }
    return block;
}
function xor(a, b) { return a.map((value, i) => value ^ b[i]); }
function lsx(block, key) { return linear(xor(block, key).map(value => PI[value])); }

export function kuznechik(key) {
    if (!(key instanceof Uint8Array) || key.length !== 32) throw new Error('Ключ «Кузнечика» должен содержать 32 байта.');
    let left = key.slice(0, 16), right = key.slice(16);
    const roundKeys = [left, right];
    for (let i = 1; i <= 32; i++) {
        const constant = new Uint8Array(16);
        constant[15] = i;
        const next = xor(lsx(left, linear(constant)), right);
        right = left;
        left = next;
        if (i % 8 === 0) roundKeys.push(left, right);
    }
    return block => {
        if (block.length !== 16) throw new Error('Некорректный размер блока.');
        let result = block.slice();
        for (let i = 0; i < 9; i++) result = lsx(result, roundKeys[i]);
        return xor(result, roundKeys[9]);
    };
}

// CTR is only used together with a separate HKDF-derived HMAC-SHA-256 key.
// A fresh random 256-bit content key is generated for every message.
export function kuznechikCTR(key, nonce, data) {
    if (nonce.length !== 16 || nonce.slice(8).some(byte => byte !== 0)) throw new Error('Некорректный счётчик.');
    const encrypt = kuznechik(key), counter = nonce.slice(), output = new Uint8Array(data.length);
    for (let offset = 0; offset < data.length; offset += 16) {
        const stream = encrypt(counter);
        for (let i = 0; i < Math.min(16, data.length - offset); i++) output[offset + i] = data[offset + i] ^ stream[i];
        for (let i = 15; i >= 8; i--) { counter[i]++; if (counter[i] !== 0) break; }
    }
    return output;
}
