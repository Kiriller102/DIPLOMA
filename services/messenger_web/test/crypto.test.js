/* global Buffer */
import test from 'node:test';
import assert from 'node:assert/strict';
import { kuznechik, kuznechikCTR } from '../src/crypto/kuznechik.js';
import { base64, createIdentity, unlockIdentity, sealMessage, openMessage, readEnvelope, signingBytes, PREFIX } from '../src/crypto/protocol.js';

const hex = value => Uint8Array.from(Buffer.from(value, 'hex'));
const key = hex('8899aabbccddeeff0011223344556677fedcba98765432100123456789abcdef');
test('Кузнечик: независимый контрольный пример RFC 7801', () => {
    const result = kuznechik(key)(hex('1122334455667700ffeeddccbbaa9988'));
    assert.equal(Buffer.from(result).toString('hex'), '7f679d90bebc24305a468d42b9d4edcd');
    assert.throws(() => kuznechik(new Uint8Array(16)));
    assert.throws(() => kuznechik(key)(new Uint8Array(15)));
});
test('Кузнечик CTR: границы блоков, бинарные данные, максимальная длина', () => {
    for (const size of [0, 1, 15, 16, 17, 31, 32, 65536]) {
        const text = Uint8Array.from({ length: size }, (_, i) => i & 255);
        const nonce = new Uint8Array(16); nonce[0] = 42;
        const encrypted = kuznechikCTR(key, nonce, text);
        assert.deepEqual(kuznechikCTR(key, nonce, encrypted), text);
        if (size) assert.notDeepEqual(encrypted, text);
    }
});

const phrase = 'Отдельная секретная фраза для теста 🔐';
const identityA = await createIdentity(1, phrase);
const identityB = await createIdentity(2, phrase + ' второй');
const identityC = await createIdentity(3, phrase + ' посторонний');
const alice = await unlockIdentity(identityA, phrase);
const bob = await unlockIdentity(identityB, phrase + ' второй');
const outsider = await unlockIdentity(identityC, phrase + ' посторонний');
const roster = [identityA, identityB];
function record(content, id = 10) { return { id, chat_id: 5, sender_id: 1, content, crypto_version: 1 }; }

test('Личные ключи: секретная фраза, повторное открытие на другом устройстве, повреждение', async () => {
    assert.equal(alice.exchange.extractable, false);
    assert.equal(alice.signing.extractable, false);
    assert.ok(!JSON.stringify(identityA).includes(phrase));
    await assert.rejects(unlockIdentity(identityA, phrase + ' ошибка'));
    await assert.rejects(unlockIdentity({ ...identityA, user_id: 4 }, phrase));
    const again = await unlockIdentity(JSON.parse(JSON.stringify(identityA)), phrase);
    assert.equal(again.identity.key_id, alice.identity.key_id);
});
for (const algorithm of ['AES', 'Kuznechik']) {
    test(`${algorithm}: оба участника читают Unicode, посторонний не читает, шифротексты различаются`, async () => {
        const text = 'Привет 👋\n100% + <script> & строки\u0000';
        const first = await sealMessage(alice, roster, text, 5, algorithm);
        const second = await sealMessage(alice, roster, text, 5, algorithm);
        assert.notEqual(first, second);
        assert.ok(!first.includes('Привет'));
        assert.equal((await openMessage(alice, identityA, record(first))).content, text);
        assert.equal((await openMessage(bob, identityA, record(first))).content, text);
        await assert.rejects(openMessage(outsider, identityA, record(first)));
        await assert.rejects(openMessage(bob, identityA, { ...record(first), chat_id: 6 }));
        await assert.rejects(openMessage(bob, identityA, { ...record(first), sender_id: 2 }));
        const edited = readEnvelope(first); edited.ciphertext = base64(new Uint8Array(20));
        await assert.rejects(openMessage(bob, identityA, record(PREFIX + JSON.stringify(edited))));
        // A correctly signed but corrupted AEAD/HMAC payload must still fail.
        edited.signature = base64(await crypto.subtle.sign({ name: 'ECDSA', hash: 'SHA-256' }, alice.signing, signingBytes(edited)));
        await assert.rejects(openMessage(bob, identityA, record(PREFIX + JSON.stringify(edited))));
    });
    test(`${algorithm}: максимальное сообщение и перенос старой истории`, async () => {
        const text = 'я'.repeat(32768);
        const encrypted = await sealMessage(alice, roster, text, 5, algorithm);
        assert.equal((await openMessage(bob, identityA, record(encrypted))).content, text);
        await assert.rejects(sealMessage(alice, roster, text + '!', 5, algorithm));
        const legacy = await sealMessage(bob, roster, 'Старая история', 5, algorithm, 1, 10);
        assert.equal((await openMessage(alice, identityB, record(legacy))).migrated, true);
        await assert.rejects(openMessage(alice, identityB, record(legacy, 11)));
    });
}
test('Нельзя отправить текст без ключей всех участников или выбрать неизвестный алгоритм', async () => {
    await assert.rejects(sealMessage(alice, [identityA, { user_id: 2 }], 'текст', 5));
    await assert.rejects(sealMessage(alice, roster, 'текст', 5, 'ECB'));
    await assert.rejects(sealMessage(alice, roster, '   ', 5));
});
test('После удаления участника новый ключ сообщения ему не выдаётся', async () => {
    const encrypted = await sealMessage(alice, [identityA], 'После удаления', 5);
    assert.equal((await openMessage(alice, identityA, record(encrypted))).content, 'После удаления');
    await assert.rejects(openMessage(bob, identityA, record(encrypted)));
});
