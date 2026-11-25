import React, { useEffect, useLayoutEffect, useState, useRef } from "react";
import styled from "styled-components";
import { api } from "../api";
import { ALGORITHMS } from '../crypto/protocol';

const decodeMessages = (messages, cryptoSession) => Promise.all(messages.map(async message => {
    try { return await cryptoSession.decrypt(message); }
    catch (err) { return { ...message, content: `Не удалось расшифровать: ${err.message}`, failed: true }; }
}));

const Messages = ({ chatID, userID, users, cryptoSession, algorithm }) => {
    const [messages, setMessages] = useState([]);
    const [error, setError] = useState('');
    const [keys, setKeys] = useState([]);
    const [migrationPending, setMigrationPending] = useState(false);
    const [migrationStatus, setMigrationStatus] = useState('');
    const [historyLoading, setHistoryLoading] = useState(Boolean(chatID));
    const [historyError, setHistoryError] = useState('');
    const [historyAttempt, setHistoryAttempt] = useState(0);
    const [nextCursor, setNextCursor] = useState(null);
    const [olderLoading, setOlderLoading] = useState(false);
    const containerRef = useRef(null);
    const requestController = useRef(null);
    const olderRequest = useRef(false);
    const scrollRestore = useRef(null);
    const scrollToBottom = useRef(false);
    const migrated = useRef(new Map());
    useEffect(() => {
        const controller = new AbortController();
        requestController.current = controller;
        olderRequest.current = false;
        scrollRestore.current = null;
        scrollToBottom.current = false;
        migrated.current.clear();
        setMessages([]);
        setKeys([]);
        setError('');
        setHistoryError('');
        setHistoryLoading(Boolean(chatID));
        setNextCursor(null);
        setOlderLoading(false);
        let timer;
        let lastID = 0;
        const poll = async () => {
            try {
                const data = await api(`/messages/longpoll/${chatID}?last_id=${lastID}`, { signal: controller.signal });
                if (controller.signal.aborted) return;
                if (data?.length) {
                    const decoded = await decodeMessages(data, cryptoSession);
                    if (controller.signal.aborted) return;
                    lastID = data[data.length - 1].id;
                    const container = containerRef.current;
                    scrollToBottom.current = container && container.scrollHeight - container.scrollTop - container.clientHeight <= 48;
                    setMessages(current => {
                        const ids = new Set(current.map(message => message.id));
                        return [...current, ...decoded.filter(message => !ids.has(message.id)).map(message => migrated.current.get(message.id) || message)];
                    });
                }
                setError('');
                timer = setTimeout(poll, 50);
            } catch (err) {
                if (err.name !== 'AbortError') {
                    if ([401, 403, 404].includes(err.status)) setError('Доступ к чату закрыт. Выберите другой чат или войдите снова.');
                    else { setError(`Не удалось получить сообщения: ${err.message}. Повторяем подключение…`); timer = setTimeout(poll, 3000); }
                }
            }
        };
        const loadHistory = async () => {
            try {
                const history = await api(`/messages/${chatID}`, { signal: controller.signal });
                if (controller.signal.aborted) return;
                const decoded = await decodeMessages(history.messages, cryptoSession);
                if (controller.signal.aborted) return;
                lastID = history.messages.at(-1)?.id || 0;
                scrollToBottom.current = true;
                setMessages(decoded);
                setNextCursor(history.next_cursor);
                setHistoryLoading(false);
                poll();
            } catch (err) {
                if (controller.signal.aborted) return;
                setHistoryLoading(false);
                setHistoryError(`Не удалось загрузить историю: ${err.message}`);
            }
        };
        if (chatID) {
            cryptoSession.chatKeys(chatID).then(data => { if (!controller.signal.aborted) setKeys(data); }).catch(err => { if (!controller.signal.aborted) setError(err.message); });
            loadHistory();
        }
        return () => { controller.abort(); clearTimeout(timer); requestController.current = null; };
    }, [chatID, cryptoSession, historyAttempt]);
    useLayoutEffect(() => {
        const container = containerRef.current;
        if (!container) return;
        const restore = scrollRestore.current;
        if (restore) {
            container.scrollTop += restore.element.getBoundingClientRect().top - container.getBoundingClientRect().top - restore.offset;
            scrollRestore.current = null;
            scrollToBottom.current = false;
        } else if (scrollToBottom.current) {
            container.scrollTop = container.scrollHeight;
            scrollToBottom.current = false;
        }
    }, [messages, nextCursor, olderLoading, historyLoading]);
    const loadPrevious = async () => {
        const controller = requestController.current;
        if (!controller || controller.signal.aborted || olderRequest.current || nextCursor === null) return;
        olderRequest.current = true;
        setOlderLoading(true);
        setHistoryError('');
        try {
            const history = await api(`/messages/${chatID}?before_id=${nextCursor}`, { signal: controller.signal });
            if (controller.signal.aborted) return;
            const decoded = await decodeMessages(history.messages, cryptoSession);
            if (controller.signal.aborted) return;
            const container = containerRef.current;
            const top = container.getBoundingClientRect().top;
            const anchor = Array.from(container.querySelectorAll('[data-message-id]')).find(element => element.getBoundingClientRect().bottom > top);
            if (anchor) scrollRestore.current = { element: anchor, offset: anchor.getBoundingClientRect().top - top };
            setMessages(current => {
                const ids = new Set(current.map(message => message.id));
                return [...decoded.filter(message => !ids.has(message.id)).map(message => migrated.current.get(message.id) || message), ...current];
            });
            setNextCursor(history.next_cursor);
        } catch (err) {
            if (!controller.signal.aborted) setHistoryError(`Не удалось загрузить предыдущие сообщения: ${err.message}`);
        } finally {
            if (!controller.signal.aborted) { olderRequest.current = false; setOlderLoading(false); }
        }
    };
    const migrate = async () => {
        if (migrationPending) return;
        setMigrationPending(true); setMigrationStatus('');
        try {
            const history = await api(`/messages/${chatID}`);
            const legacy = history.messages.filter(message => message.crypto_version === 0);
            for (const [index, message] of legacy.entries()) {
                setMigrationStatus(`Шифруем старую историю: ${index + 1} из ${legacy.length}…`);
                const encrypted = await cryptoSession.migrate(message, algorithm);
                migrated.current.set(message.id, encrypted);
                setMessages(current => current.map(item => item.id === message.id ? encrypted : item));
            }
            setMigrationStatus('Старая история зашифрована.');
        } catch (err) { setMigrationStatus(`Перенос остановлен: ${err.message}. Уже перенесённые сообщения сохранены.`); }
        finally { setMigrationPending(false); }
    };
    return <StyledMessages ref={containerRef} aria-live="polite">
        {chatID && <details className="keys"><summary>Ключи участников · сверить отпечатки</summary>
            <p>Сверьте эти отпечатки с участниками по другому каналу связи, особенно при первом общении.</p>
            {keys.map(key => <p key={key.user_id}>{users.find(user => user.id === key.user_id)?.username || `Пользователь ${key.user_id}`}<br /><code>{key.key_id?.match(/.{1,8}/g)?.join(' ') || 'Ещё не настроил шифрование'}</code></p>)}
        </details>}
        {error && <p role="alert">{error}</p>}
        {historyError && <p role="alert">{historyError}</p>}
        {historyError && !messages.length && <button onClick={() => setHistoryAttempt(attempt => attempt + 1)}>Повторить загрузку истории</button>}
        {messages.some(message => message.legacy) && <div className="legacy" role="status">
            <p>В чате есть старая история, которая хранится открытым текстом. Для переноса все участники должны настроить шифрование.</p>
            <button disabled={migrationPending} onClick={migrate}>Зашифровать старую историю</button>
            <small>Алгоритм переноса: {ALGORITHMS[algorithm]}</small>
        </div>}
        {migrationStatus && <p role="status">{migrationStatus}</p>}
        {historyLoading && <p role="status">Загрузка истории…</p>}
        {nextCursor !== null && <button disabled={olderLoading} onClick={loadPrevious}>{olderLoading ? 'Загрузка…' : 'Загрузить предыдущие'}</button>}
        {!historyLoading && !historyError && !messages.length && <div className="welcome"><h3>{chatID ? 'Начните беседу' : 'Выберите чат'}</h3><p>Создайте чат и пригласите другого пользователя.</p></div>}
        {messages.map(message => <div key={message.id} data-message-id={message.id} className={`message ${message.sender_id === userID ? 'own' : ''}`}>
            <small>{users.find(user => user.id === message.sender_id)?.username || `Пользователь ${message.sender_id}`} · {new Date(message.sent_at).toLocaleTimeString([], { hour: '2-digit', minute: '2-digit' })} · {message.legacy ? 'Без шифрования: старая история' : message.failed ? 'Ошибка проверки' : ALGORITHMS[message.algorithm]}{message.migrated ? ' · перенесено участником из старой истории' : ''}</small>
            <div>{message.content}</div>
        </div>)}
    </StyledMessages>;
};

const StyledMessages = styled.div`
    flex: auto;
    overflow-y: auto;
    overflow-anchor: none;
    padding-top: 10px;
    background: rgba(0, 0, 0, 0);
    border-radius: 10px;
    display: flex;
    flex-direction: column;
    width: 100%;
    .keys, .legacy { padding: 10px; border: 1px solid #666879; border-radius: 8px; margin-bottom: 10px; color: #eee; }
    .keys code { overflow-wrap: anywhere; font-size: 12px; }
    .legacy small { display: block; margin-top: 6px; }

    .own { align-self: flex-end; }
    small { color: #b9bdec; }
    .message {
        flex-shrink: 0;
        background: rgba(247, 197, 159, .1);
        color: rgb(255, 255, 255);
        padding: 8px 12px;
        border-radius: 6px;
        margin-bottom: 5px;
        margin-right: 10px;
        width: fit-content;
        max-width: 100%;
        white-space: pre-wrap;
        word-wrap: break-word;
        word-break: break-word;
    }

    .welcome {
        text-align: center;
        color: #bbb;
        padding: 20px;
    }

    .welcome h3 {
        align-self: center;
        margin-top: auto;
        font-size: 18px;
        font-weight: bold;
    }

    .welcome p {
        margin-top: 15px;
        font-size: 14px;
    }
`;

export default Messages;
