import React, { useEffect, useState, useRef } from "react";
import styled from "styled-components";

const Messages = ({ chatID }) => {
    const [messages, setMessages] = useState([]);
    const [lastId, setLastId] = useState(0);
    const messagesEndRef = useRef(null);
    const currentChatIdRef = useRef(chatID);
    const [showWelcomeMessage, setShowWelcomeMessage] = useState(true);

    const fetchMessages = async (abortController) => {
        if (!chatID) return;

        try {
            const response = await fetch(`http://localhost:8080/messages/longpoll/${chatID}?last_id=${lastId}`, {
                signal: abortController.signal,
            });

            if (response.status === 204) {
                if (currentChatIdRef.current === chatID) {
                    setTimeout(fetchMessages, 1000, abortController);
                }
                return;
            }

            const data = await response.json();

            if (Array.isArray(data)) {

                setMessages(data);


                setLastId(data[data.length - 1]?.id || lastId);
            }

        } catch (error) {
            if (error.name !== "AbortError") {
                console.error("Ошибка при получении сообщений:", error);

                setTimeout(fetchMessages, 5000, abortController);
            }
        }
    };

    useEffect(() => {
        setShowWelcomeMessage(false);
        currentChatIdRef.current = chatID;

        setMessages([]);
        setLastId(0);

        const abortController = new AbortController();
        fetchMessages(abortController);

        const welcomeMessageTimeout = setTimeout(() => {
            setShowWelcomeMessage(true);
        }, 200);

        return () => {
            abortController.abort();
            clearTimeout(welcomeMessageTimeout);
        };
    }, [chatID]);

    useEffect(() => {
        const abortController = new AbortController();
        fetchMessages(abortController);

        return () => {
            abortController.abort();
        };
    }, [lastId]);


    useEffect(() => {
        if (messagesEndRef.current) {
            messagesEndRef.current.scrollIntoView({ behavior: "smooth" });
        }
    }, [messages]);

    return (
        <StyledMessages>
            {messages.length === 0  && showWelcomeMessage  ? (
                <WelcomeMessage />
            ) : (
                messages.map((message) => (
                    <div key={message.id} className="message">
                        {decodeURIComponent(message.content)}
                    </div>
                ))
            )}
            <div ref={messagesEndRef} />
        </StyledMessages>
    );
};

const WelcomeMessage = () => (
    <div className="welcome">
        <h3>Добро пожаловать в чат!</h3>
        <p>Начните беседу, отправив сообщение.</p>
    </div>
);

const StyledMessages = styled.div`
    flex: auto;
    overflow-y: auto;
    padding-top: 10px;
    background: rgba(0, 0, 0, 0);
    border-radius: 10px;
    display: flex;
    flex-direction: column;
    width: 100%;

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
