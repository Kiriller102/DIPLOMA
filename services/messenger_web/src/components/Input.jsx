import React, { useState, useRef } from 'react';
import styled from 'styled-components';

const Input = ({ chatID }) => {
    const [message, setMessage] = useState('');
    const textareaRef = useRef(null);

    const handleSubmit = async (e) => {
        e.preventDefault();
        if (message.trim() && chatID) {
            try {
                const response = await fetch('http://localhost:8080/messages', {
                    method: 'POST',
                    headers: {
                        'Content-Type': 'application/json',
                    },
                    body: JSON.stringify({
                        content: encodeURIComponent(message),
                        sender_id: 1,
                        chat_id: chatID,
                        encryption_algorithm: "AES"
                    }),
                });

                if (response.ok) {
                    setMessage('');
                    textareaRef.current.style.height = "auto";
                } else {
                    console.error('Failed to send message:', response.statusText);
                }
            } catch (error) {
                console.error('Error sending message:', error);
            }
        }
    };

    const handleChange = (event) => {
        setMessage(event.target.value);
        textareaRef.current.style.height = "auto";
        textareaRef.current.style.height = Math.min(textareaRef.current.scrollHeight, 10 * 24) + "px";
    };

    const handleKeyDown = (event) => {
        if (event.key === 'Enter' && !event.shiftKey) {
            event.preventDefault();
            handleSubmit(event);
        }
    };

    return (
        <StyledWrapper>
            <div className="input-container">
                <textarea
                    ref={textareaRef}
                    className="input"
                    placeholder="Write a message..."
                    value={message}
                    onChange={handleChange}
                    onKeyDown={handleKeyDown}
                    rows="1"
                />
                <button className="sendButton" onClick={handleSubmit}>
                    <svg height={24} width={24} viewBox="0 0 24 24" xmlns="http://www.w3.org/2000/svg">
                        <path d="M0 0h24v24H0z" fill="none"/>
                        <path d="M16.172 11l-5.364-5.364 1.414-1.414L20 12l-7.778 7.778-1.414-1.414L16.172 13H4v-2z" fill="currentColor"/>
                    </svg>
                </button>
            </div>
        </StyledWrapper>
    );
};

const StyledWrapper = styled.div`
    display: flex;
    justify-content: flex-end;
    width: 100%;
    margin-right: 10px;
    margin-bottom: 10px;

    .input-container {
        display: flex;
        align-items: center;
        width: 100%;
        background-color: rgb(28, 28, 30);
        border-radius: 20px;
        padding: 5px;
        box-shadow: 0 0 .4vw rgba(0, 0, 0, 0.5);
    }

    .input {
        font-family: -apple-system, BlinkMacSystemFont, "Segoe UI", "Roboto", "Oxygen", "Ubuntu", "Cantarell", "Fira Sans", "Droid Sans", "Helvetica Neue", sans-serif;
        font-weight: 300;
        font-size: 1rem;
        position: relative;
        flex-grow: 1;
        color: #fff;
        background-color: transparent;
        border: none;
        outline: none;
        padding: 10px;
        resize: none;
        height: auto;
        line-height: 1.2;
        overflow-y: auto;
        box-sizing: border-box;
        min-height: 1.2rem;
        max-height: 10rem;
        width: 100%;
        border-radius: 20px 0 0 20px;
    }

    .input-container:hover {
        box-shadow: 0 0 0 .15vw rgba(247, 197, 159, .1);
    }

    .input-container:focus {
        box-shadow: 0 0 0 .15vw rgb(247, 197, 159);
    }
    
    .sendButton {
        background: #7079f0;
        color: white;
        font-family: inherit;
        font-size: 15px;
        font-weight: 500;
        border: none;
        cursor: pointer;
        display: flex;
        align-items: center;
        align-self: end;
        margin-left: auto;
        justify-content: center;
        padding: 0.6em;
        border-radius: 0 20px 20px 0;
        box-shadow: inset 0 0 1.6em -0.6em #714da6;
        transition: background 0.3s ease;
    }

    .sendButton:hover {
        background: #5566c2;
    }

    .sendButton:active {
        background: #4051a2;
    }
`;

export default Input;
