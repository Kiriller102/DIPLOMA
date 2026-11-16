import { kuznechikCTR } from './kuznechik.js';

export const PREFIX = 'e2ee:v1:';
export const ITERATIONS = 600000;
export const ALGORITHMS = { AES: 'AES-256-GCM', Kuznechik: 'Кузнечик · CTR + HMAC-SHA-256' };
const textEncoder = new TextEncoder();
const textDecoder = new TextDecoder('utf-8', { fatal: true });
const encode = value => textEncoder.encode(value);
const random = size => crypto.getRandomValues(new Uint8Array(size));
const bytes = value => new Uint8Array(value);
export function base64(value) {
    let string = '';
    for (const byte of bytes(value)) string += String.fromCharCode(byte);
    return btoa(string);
}
export function unbase64(value) {
    const result = Uint8Array.from(atob(value), char => char.charCodeAt(0));
    if (base64(result) !== value) throw new Error('Некорректные зашифрованные данные.');
    return result;
}
const concat = (...arrays) => {
    const output = new Uint8Array(arrays.reduce((length, array) => length + array.length, 0));
    let offset = 0;
    for (const array of arrays) { output.set(array, offset); offset += array.length; }
    return output;
};
export async function hashText(value) { return base64(await crypto.subtle.digest('SHA-256', encode(value))); }
export function requireCrypto() {
    if (!globalThis.crypto?.subtle) throw new Error('Для шифрования откройте приложение по HTTPS или через localhost.');
}
async function fingerprint(exchange, signing) {
    return Array.from(bytes(await crypto.subtle.digest('SHA-256', concat(unbase64(exchange), unbase64(signing)))), b => b.toString(16).padStart(2, '0')).join('');
}
export async function checkIdentity(identity) {
    if (!identity || !Number.isSafeInteger(identity.user_id) || identity.user_id <= 0 ||
        await fingerprint(identity.exchange_key, identity.signing_key) !== identity.key_id) throw new Error('Некорректный открытый ключ участника.');
    await crypto.subtle.importKey('raw', unbase64(identity.exchange_key), { name: 'ECDH', namedCurve: 'P-256' }, false, []);
    await crypto.subtle.importKey('raw', unbase64(identity.signing_key), { name: 'ECDSA', namedCurve: 'P-256' }, false, ['verify']);
}
async function vaultKey(phrase, salt, usages) {
    const material = await crypto.subtle.importKey('raw', encode(phrase), 'PBKDF2', false, ['deriveKey']);
    return crypto.subtle.deriveKey({ name: 'PBKDF2', salt, iterations: ITERATIONS, hash: 'SHA-256' }, material, { name: 'AES-GCM', length: 256 }, false, usages);
}
const vaultAAD = identity => encode(JSON.stringify(['DIPLOMA:vault:v1', identity.user_id, identity.key_id]));

export async function createIdentity(userID, phrase) {
    requireCrypto();
    if (Array.from(phrase).length < 16) throw new Error('Секретная фраза должна содержать минимум 16 символов.');
    const exchange = await crypto.subtle.generateKey({ name: 'ECDH', namedCurve: 'P-256' }, true, ['deriveBits']);
    const signing = await crypto.subtle.generateKey({ name: 'ECDSA', namedCurve: 'P-256' }, true, ['sign', 'verify']);
    const identity = {
        user_id: userID,
        exchange_key: base64(await crypto.subtle.exportKey('raw', exchange.publicKey)),
        signing_key: base64(await crypto.subtle.exportKey('raw', signing.publicKey)),
    };
    identity.key_id = await fingerprint(identity.exchange_key, identity.signing_key);
    const salt = random(16), nonce = random(12);
    const key = await vaultKey(phrase, salt, ['encrypt']);
    const secret = encode(JSON.stringify({
        exchange: base64(await crypto.subtle.exportKey('pkcs8', exchange.privateKey)),
        signing: base64(await crypto.subtle.exportKey('pkcs8', signing.privateKey)),
    }));
    try {
        identity.encrypted_private_keys = base64(await crypto.subtle.encrypt({ name: 'AES-GCM', iv: nonce, additionalData: vaultAAD(identity) }, key, secret));
    } finally { secret.fill(0); }
    return { ...identity, salt: base64(salt), nonce: base64(nonce), iterations: ITERATIONS };
}

export async function unlockIdentity(identity, phrase) {
    requireCrypto();
    await checkIdentity(identity);
    if (identity.iterations !== ITERATIONS || unbase64(identity.salt).length !== 16 || unbase64(identity.nonce).length !== 12) throw new Error('Неподдерживаемый формат ключей.');
    const key = await vaultKey(phrase, unbase64(identity.salt), ['decrypt']);
    let secret;
    try {
        secret = bytes(await crypto.subtle.decrypt({ name: 'AES-GCM', iv: unbase64(identity.nonce), additionalData: vaultAAD(identity) }, key, unbase64(identity.encrypted_private_keys)));
        const data = JSON.parse(textDecoder.decode(secret));
        const exchange = await crypto.subtle.importKey('pkcs8', unbase64(data.exchange), { name: 'ECDH', namedCurve: 'P-256' }, false, ['deriveBits']);
        const signing = await crypto.subtle.importKey('pkcs8', unbase64(data.signing), { name: 'ECDSA', namedCurve: 'P-256' }, false, ['sign']);
        const challenge = random(32);
        const signature = await crypto.subtle.sign({ name: 'ECDSA', hash: 'SHA-256' }, signing, challenge);
        const signingPublic = await crypto.subtle.importKey('raw', unbase64(identity.signing_key), { name: 'ECDSA', namedCurve: 'P-256' }, false, ['verify']);
        if (!await crypto.subtle.verify({ name: 'ECDSA', hash: 'SHA-256' }, signingPublic, signature, challenge)) throw new Error('Ключ подписи не совпадает.');
        const probe = await crypto.subtle.generateKey({ name: 'ECDH', namedCurve: 'P-256' }, false, ['deriveBits']);
        const publicExchange = await crypto.subtle.importKey('raw', unbase64(identity.exchange_key), { name: 'ECDH', namedCurve: 'P-256' }, false, []);
        const first = bytes(await crypto.subtle.deriveBits({ name: 'ECDH', public: probe.publicKey }, exchange, 256));
        const second = bytes(await crypto.subtle.deriveBits({ name: 'ECDH', public: publicExchange }, probe.privateKey, 256));
        if (!first.every((byte, i) => byte === second[i])) throw new Error('Ключ обмена не совпадает.');
        return { identity, exchange, signing };
    } catch {
        throw new Error('Неверная секретная фраза или повреждённая копия ключей.');
    } finally { secret?.fill(0); }
}

const header = e => [e.v, e.algorithm, e.chat_id, e.sender_id, e.signer_id, e.message_id, e.legacy_id];
const aad = e => encode(JSON.stringify(header(e)));
const wrapAAD = (e, r) => encode(JSON.stringify([...header(e), r.user_id, r.key_id]));
export function signingBytes(e) {
    return encode(JSON.stringify([...header(e), e.ephemeral_key, e.nonce, e.ciphertext, e.tag,
        e.recipients.map(r => [r.user_id, r.key_id, r.nonce, r.key])]));
}
async function derive(seed, salt, info, name, usages) {
    const material = await crypto.subtle.importKey('raw', seed, 'HKDF', false, ['deriveKey']);
    const algorithm = name === 'HMAC' ? { name, hash: 'SHA-256', length: 256 } : { name, length: 256 };
    return crypto.subtle.deriveKey({ name: 'HKDF', hash: 'SHA-256', salt, info: encode(info) }, material, algorithm, false, usages);
}
async function kuzKey(seed, e) {
    const material = await crypto.subtle.importKey('raw', seed, 'HKDF', false, ['deriveBits']);
    return bytes(await crypto.subtle.deriveBits({ name: 'HKDF', hash: 'SHA-256', salt: unbase64(e.message_id), info: encode('DIPLOMA:kuznechik:encryption:v1') }, material, 256));
}
const macKey = (seed, e) => derive(seed, unbase64(e.message_id), 'DIPLOMA:kuznechik:authentication:v1', 'HMAC', ['sign', 'verify']);
async function wrappingKey(privateKey, publicRaw, e) {
    const publicKey = await crypto.subtle.importKey('raw', publicRaw, { name: 'ECDH', namedCurve: 'P-256' }, false, []);
    const shared = bytes(await crypto.subtle.deriveBits({ name: 'ECDH', public: publicKey }, privateKey, 256));
    try { return await derive(shared, unbase64(e.message_id), 'DIPLOMA:message-key-wrap:v1', 'AES-GCM', ['encrypt', 'decrypt']); }
    finally { shared.fill(0); }
}

export async function sealMessage(unlocked, identities, text, chatID, algorithm = 'AES', senderID = unlocked.identity.user_id, legacyID = 0) {
    requireCrypto();
    const plaintext = encode(text);
    if (!text.trim() || plaintext.length > 65536) throw new Error('Сообщение должно содержать от 1 до 65536 байт.');
    if (!Object.hasOwn(ALGORITHMS, algorithm)) throw new Error('Выберите поддерживаемый алгоритм.');
    if (!identities.length || identities.length > 100 || identities.some(i => !i.key_id)) throw new Error('Все участники должны сначала настроить шифрование в своём аккаунте.');
    const sorted = [...identities].sort((a, b) => a.user_id - b.user_id);
    if (new Set(sorted.map(i => i.user_id)).size !== sorted.length || !sorted.some(i => i.key_id === unlocked.identity.key_id && i.user_id === unlocked.identity.user_id)) throw new Error('Некорректный список участников.');
    await Promise.all(sorted.map(checkIdentity));
    const e = { v: 1, algorithm, chat_id: chatID, sender_id: senderID, signer_id: unlocked.identity.user_id,
        message_id: base64(random(16)), legacy_id: legacyID, ephemeral_key: '', nonce: '', ciphertext: '', tag: '', recipients: [] };
    const seed = random(32);
    try {
        if (algorithm === 'AES') {
            const nonce = random(12);
            const key = await crypto.subtle.importKey('raw', seed, 'AES-GCM', false, ['encrypt']);
            e.nonce = base64(nonce);
            e.ciphertext = base64(await crypto.subtle.encrypt({ name: 'AES-GCM', iv: nonce, additionalData: aad(e) }, key, plaintext));
        } else {
            const nonce = new Uint8Array(16); nonce.set(random(8));
            const key = await kuzKey(seed, e);
            let ciphertext;
            try { ciphertext = kuznechikCTR(key, nonce, plaintext); } finally { key.fill(0); }
            e.nonce = base64(nonce); e.ciphertext = base64(ciphertext);
            e.tag = base64(await crypto.subtle.sign('HMAC', await macKey(seed, e), concat(aad(e), nonce, ciphertext)));
        }
        const ephemeral = await crypto.subtle.generateKey({ name: 'ECDH', namedCurve: 'P-256' }, false, ['deriveBits']);
        e.ephemeral_key = base64(await crypto.subtle.exportKey('raw', ephemeral.publicKey));
        for (const identity of sorted) {
            const r = { user_id: identity.user_id, key_id: identity.key_id, nonce: base64(random(12)), key: '' };
            const key = await wrappingKey(ephemeral.privateKey, unbase64(identity.exchange_key), e);
            r.key = base64(await crypto.subtle.encrypt({ name: 'AES-GCM', iv: unbase64(r.nonce), additionalData: wrapAAD(e, r) }, key, seed));
            e.recipients.push(r);
        }
        e.signature = base64(await crypto.subtle.sign({ name: 'ECDSA', hash: 'SHA-256' }, unlocked.signing, signingBytes(e)));
        return PREFIX + JSON.stringify(e);
    } finally { seed.fill(0); plaintext.fill(0); }
}

export function readEnvelope(content) {
    if (!content.startsWith(PREFIX) || content.length > 256 * 1024) throw new Error('Неподдерживаемый формат сообщения.');
    const e = JSON.parse(content.slice(PREFIX.length));
    if (e.v !== 1 || !Object.hasOwn(ALGORITHMS, e.algorithm) || !Array.isArray(e.recipients) || !e.recipients.length || e.recipients.length > 100) throw new Error('Неподдерживаемый формат сообщения.');
    return e;
}

export async function openMessage(unlocked, signer, message) {
    const e = readEnvelope(message.content);
    if (e.chat_id !== message.chat_id || e.sender_id !== message.sender_id || e.signer_id !== signer.user_id ||
        (e.legacy_id !== 0 && e.legacy_id !== message.id) || (e.legacy_id === 0 && e.signer_id !== e.sender_id)) throw new Error('Подменены сведения о сообщении.');
    await checkIdentity(signer);
    const signingKey = await crypto.subtle.importKey('raw', unbase64(signer.signing_key), { name: 'ECDSA', namedCurve: 'P-256' }, false, ['verify']);
    if (!await crypto.subtle.verify({ name: 'ECDSA', hash: 'SHA-256' }, signingKey, unbase64(e.signature), signingBytes(e))) throw new Error('Подпись сообщения не прошла проверку.');
    const recipient = e.recipients.find(r => r.user_id === unlocked.identity.user_id && r.key_id === unlocked.identity.key_id);
    if (!recipient) throw new Error('Это сообщение зашифровано до вашего приглашения в чат или для другого ключа.');
    const key = await wrappingKey(unlocked.exchange, unbase64(e.ephemeral_key), e);
    const seed = bytes(await crypto.subtle.decrypt({ name: 'AES-GCM', iv: unbase64(recipient.nonce), additionalData: wrapAAD(e, recipient) }, key, unbase64(recipient.key)));
    let plaintext;
    try {
        if (seed.length !== 32) throw new Error('Некорректный ключ сообщения.');
        const nonce = unbase64(e.nonce), ciphertext = unbase64(e.ciphertext);
        if (e.algorithm === 'AES') {
            const contentKey = await crypto.subtle.importKey('raw', seed, 'AES-GCM', false, ['decrypt']);
            plaintext = bytes(await crypto.subtle.decrypt({ name: 'AES-GCM', iv: nonce, additionalData: aad(e) }, contentKey, ciphertext));
        } else {
            if (!await crypto.subtle.verify('HMAC', await macKey(seed, e), unbase64(e.tag), concat(aad(e), nonce, ciphertext))) throw new Error('Нарушена целостность сообщения.');
            const contentKey = await kuzKey(seed, e);
            try { plaintext = kuznechikCTR(contentKey, nonce, ciphertext); } finally { contentKey.fill(0); }
        }
        return { ...message, content: textDecoder.decode(plaintext), algorithm: e.algorithm, migrated: e.legacy_id !== 0, crypto_version: 1 };
    } finally { seed.fill(0); plaintext?.fill(0); }
}
