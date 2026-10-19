import React, { useState, useEffect } from 'react';
import styled, {keyframes, css} from 'styled-components';

const ChatContainer = ({ isChatVisible, setChatID  }) => {
    const [chatList, setChatList] = useState([]);
    const [clickedChat, setClickedChat] = useState(null);

    useEffect(() => {
        const fetchChats = async () => {
            try {
                const response = await fetch('http://localhost:8080/chat_participant/chats/1');
                const data = await response.json();

                if (Array.isArray(data)) {
                    setChatList(data);
                } else {
                    console.error('Fetched data is not an array:', data);
                }
            } catch (error) {
                console.error('Error fetching chat list:', error);
            }
        };

        fetchChats();
    }, []);

    const handleChatClick = (chat) => {
        console.log("Chat click:", chat);
        setClickedChat(chat.id);
        setChatID(chat.id);
        setTimeout(() => setClickedChat(null), 300);
    }

    return (
        <StyledChatBox isChatVisible={isChatVisible}>
            <StyledChatList>
                {chatList.map(chat => (
                    <StyledChat
                        key={chat.id}
                        onClick={() => handleChatClick(chat)}
                        isClicked={clickedChat === chat.id}
                    >
                        {/*<h1>{chat.id}</h1>*/}
                        <h3> {chat.name}</h3>
                        {/*<h3>{chat.name}</h3>*/}
                        {/*<p>{chat.message}</p>*/}
                    </StyledChat>
                ))}
            </StyledChatList>
        </StyledChatBox>
    );
};

const StyledChatBox = styled.div`
    width: min(400px, calc(100vw - 60px));
    background-color: #f0f0f0;
    border-radius: 15px;
    padding: 20px;
    margin: 10px 10px 10px calc((min(400px, 100vw - 60px) + 30px) * -1);
    overflow-y: auto;
    box-shadow: 0 4px 8px rgba(0, 0, 0, 0.1);
    transition: transform 0.3s ease;
    transform: ${({isChatVisible}) => (isChatVisible ? 'translateX(0)' : 'translateX(+100%)')};
`;

const StyledChatList = styled.div`
    display: flex;
    flex-direction: column;
    gap: 15px;
`;

const clickAnimation = keyframes`
    0% { transform: scale(1); }
    50% { transform: scale(1.05); }
    100% { transform: scale(1); }
`;

const StyledChat = styled.div`
    background-color: white;
    padding: 10px;
    border-radius: 10px;
    box-shadow: 0 2px 4px rgba(0, 0, 0, 0.1);
    cursor: pointer;
    ${({ isClicked }) => isClicked && css`animation: ${clickAnimation} 0.3s;`}

    h3 {
        margin: 0;
        font-size: 16px;
        font-weight: bold;
        color: #1a1a1a;
    }

    p {
        margin: 5px 0;
        font-size: 14px;
        color: #666;
    }
`;

export default ChatContainer;