import { api } from '../api.js';
import { checkIdentity, hashText, openMessage, readEnvelope, sealMessage } from './protocol.js';

async function pinIdentity(identity) {
    await checkIdentity(identity);
    const name = `messenger-e2ee-key:${identity.user_id}`;
    const previous = localStorage.getItem(name);
    if (previous && previous !== identity.key_id) throw new Error(`Ключ пользователя ${identity.user_id} изменился. Сверьте отпечаток с участником перед продолжением.`);
    localStorage.setItem(name, identity.key_id);
}

export async function createCryptoSession(unlocked) {
    await pinIdentity(unlocked.identity);
    const known = new Map([[unlocked.identity.user_id, unlocked.identity]]);
    const userID = unlocked.identity.user_id;
    let refreshing;
    const loadKeys = async () => {
        const keys = await api('/crypto/keys');
        for (const identity of keys) { await pinIdentity(identity); known.set(identity.user_id, identity); }
    };
    const refreshKeys = () => {
        if (!refreshing) refreshing = loadKeys().finally(() => { refreshing = null; });
        return refreshing;
    };
    const chatKeys = async chatID => {
        const keys = await api(`/crypto/chat/${chatID}`);
        for (const identity of keys) {
            if (identity.key_id) { await pinIdentity(identity); known.set(identity.user_id, identity); }
        }
        return keys;
    };
    const encrypt = async (text, chatID, algorithm, senderID = userID, legacyID = 0) =>
        sealMessage(unlocked, await chatKeys(chatID), text, chatID, algorithm, senderID, legacyID);
    const decrypt = async message => {
        // Old plaintext is explicitly labelled; invalid encrypted content is never
        // downgraded to plaintext, even if a server changes crypto_version.
        if (message.crypto_version === 0 && !message.content.startsWith('e2ee:v1:')) return { ...message, legacy: true };
        const envelope = readEnvelope(message.content);
        if (!known.has(envelope.signer_id)) await refreshKeys();
        const signer = known.get(envelope.signer_id);
        if (!signer) throw new Error('Не найден ключ подписи отправителя.');
        return openMessage(unlocked, signer, message);
    };
    const migrate = async (message, algorithm) => {
        const content = await encrypt(message.content, message.chat_id, algorithm, message.sender_id, message.id);
        const { message: encrypted } = await api(`/messages/${message.id}/encrypt`, {
            method: 'POST', body: JSON.stringify({ content, previous_hash: await hashText(message.content) }),
        });
        return decrypt(encrypted);
    };
    return { userID, identity: unlocked.identity, chatKeys, encrypt, decrypt, migrate };
}
