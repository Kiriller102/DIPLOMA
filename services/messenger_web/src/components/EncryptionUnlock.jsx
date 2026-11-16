import { useEffect, useState } from 'react';
import styled from 'styled-components';
import { api } from '../api';
import { createIdentity, requireCrypto, unlockIdentity } from '../crypto/protocol';
import { createCryptoSession } from '../crypto/session';

export default function EncryptionUnlock({ user, onUnlocked, logout }) {
    const [identity, setIdentity] = useState(null);
    const [loading, setLoading] = useState(true);
    const [available, setAvailable] = useState(false);
    const [error, setError] = useState('');
    const [phrase, setPhrase] = useState('');
    const [confirmation, setConfirmation] = useState('');
    const [pending, setPending] = useState(false);
    const [retry, setRetry] = useState(0);
    useEffect(() => {
        const controller = new AbortController();
        setLoading(true); setAvailable(false); setError('');
        const load = async () => {
            try {
                requireCrypto();
                const stored = await api('/crypto/identity', { signal: controller.signal });
                if (!controller.signal.aborted) { setIdentity(stored); setAvailable(true); }
            } catch (err) {
                if (err.status === 404 && !controller.signal.aborted) { setIdentity(null); setAvailable(true); }
                if (err.name !== 'AbortError' && err.status !== 404) setError(err.message);
            } finally { if (!controller.signal.aborted) setLoading(false); }
        };
        load();
        return () => controller.abort();
    }, [user.id, retry]);
    const submit = async event => {
        event.preventDefault();
        if (pending) return;
        if (!identity && phrase !== confirmation) { setError('Секретные фразы не совпадают.'); return; }
        setPending(true); setError('');
        try {
            const stored = identity || await createIdentity(user.id, phrase);
            if (!identity) {
                try { await api('/crypto/identity', { method: 'POST', body: JSON.stringify(stored) }); }
                catch (err) {
                    if (err.status === 409) { setIdentity(await api('/crypto/identity')); throw new Error('Ключи уже настроены в другом окне. Введите созданную там секретную фразу.'); }
                    throw err;
                }
            }
            // Remember a successful setup even if unlocking/pinning later fails.
            setIdentity(stored);
            const session = await createCryptoSession(await unlockIdentity(stored, phrase));
            setPhrase(''); setConfirmation('');
            onUnlocked(session);
        } catch (err) { setError(err.message); }
        finally { setPending(false); }
    };
    return <Page><Panel>
        <h1>Сквозное шифрование</h1>
        <p>{user.username}</p>
        {loading ? <p role="status">Проверяем ключи…</p> : available ? <>
            <h2>{identity ? 'Открыть переписку' : 'Защитить переписку'}</h2>
            <p>{identity ? 'Введите секретную фразу шифрования. На другом устройстве используйте эту же фразу.' : 'Создайте отдельную секретную фразу. Она защитит личные ключи и не будет отправлена серверу. Используйте фразу, отличающуюся от пароля для входа.'}</p>
            {!identity && <p>Сохраните фразу: восстановить её и расшифровать сообщения без неё невозможно.</p>}
            <form onSubmit={submit}>
                <label>Секретная фраза<input aria-label="Секретная фраза" type="password" autoComplete="off" minLength={identity ? 1 : 16} maxLength={1024} required disabled={pending} value={phrase} onChange={event => setPhrase(event.target.value)} /></label>
                {!identity && <label>Повторите секретную фразу<input aria-label="Повторите секретную фразу" type="password" autoComplete="off" required disabled={pending} value={confirmation} onChange={event => setConfirmation(event.target.value)} /></label>}
                <button type="submit" disabled={pending}>{pending ? 'Открываем ключи…' : identity ? 'Открыть переписку' : 'Настроить шифрование'}</button>
            </form>
        </> : null}
        {error && <p role="alert">{error} <button onClick={() => setRetry(value => value + 1)} disabled={pending}>Повторить проверку</button></p>}
        <button onClick={logout} disabled={pending}>Выйти</button>
    </Panel></Page>;
}
const Page = styled.main`
    min-height: 100dvh; display: grid; place-items: center; padding: 24px; box-sizing: border-box; background: #20212a; color: #f2f2f5;
`;
const Panel = styled.section`
    width: min(100%, 480px); border: 1px solid #414252; border-radius: 18px; padding: 28px; box-sizing: border-box; background: #2c2d38;
    h1 { font-size: 25px; } h2 { font-size: 20px; } p { line-height: 1.5; }
    form, label { display: grid; gap: 8px; } form { gap: 16px; margin: 20px 0; }
    input { min-width: 0; padding: 10px; border: 1px solid #666879; border-radius: 8px; background: #20212a; color: white; font: inherit; }
    button[type=submit] { background: #7079f0; color: white; }
`;
