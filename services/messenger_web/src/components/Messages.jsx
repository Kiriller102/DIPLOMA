import React, { useEffect, useState, useRef } from "react";
import styled from "styled-components";
import { api } from "../api";

const Messages = ({ chatID, userID, users }) => {
    const [messages, setMessages] = useState([]);
    const [error, setError] = useState('');
    const endRef = useRef(null);
    useEffect(() => {
        if (!chatID) return;
        const controller = new AbortController();
        let timer;
        let lastID = 0;
        const poll = async () => {
            try {
                const data = await api(`/messages/longpoll/${chatID}?last_id=${lastID}`, { signal: controller.signal });
                if (controller.signal.aborted) return;
                if (data?.length) {
                    lastID = data[data.length - 1].id;
                    setMessages(current => {
                        const ids = new Set(current.map(message => message.id));
                        return [...current, ...data.filter(message => !ids.has(message.id))];
                    });
                }
                setError('');
                timer = setTimeout(poll, 50);
            } catch (err) {
                if (err.name !== 'AbortError') {
                    setError(`Не удалось получить сообщения: ${err.message}. Повторяем подключение…`);
                    timer = setTimeout(poll, 3000);
                }
            }
        };
        poll();
        return () => { controller.abort(); clearTimeout(timer); };
    }, [chatID]);
    useEffect(() => { endRef.current?.scrollIntoView({ behavior: 'smooth' }); }, [messages]);
    return <StyledMessages aria-live="polite">
        {error && <p role="alert">{error}</p>}
        {!messages.length && <div className="welcome"><h3>{chatID ? 'Начните беседу' : 'Выберите чат'}</h3><p>Выберите другого пользователя во второй вкладке, чтобы проверить обмен сообщениями.</p></div>}
        {messages.map(message => <div key={message.id} className={`message ${message.sender_id === userID ? 'own' : ''}`}>
            <small>{users.find(user => user.id === message.sender_id)?.username || `Пользователь ${message.sender_id}`} · {new Date(message.sent_at).toLocaleTimeString([], { hour: '2-digit', minute: '2-digit' })}</small>
            <div>{message.content}</div>
        </div>)}
        <div ref={endRef} />
    </StyledMessages>;
};

const StyledMessages = styled.div`
    flex: auto;
    overflow-y: auto;
    padding-top: 10px;
    background: rgba(0, 0, 0, 0);
    border-radius: 10px;
    display: flex;
    flex-direction: column;
    width: 100%;

    .own { align-self: flex-end; }
    small { color: #b9bdec; }
    .message {
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
