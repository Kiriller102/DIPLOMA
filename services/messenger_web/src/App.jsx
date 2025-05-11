import React, {useState} from 'react';
import Header from './components/Header';
import { useAuth0 } from '@auth0/auth0-react';
import Background from './components/Background';
import ChatContainer from './components/Chats';
import Input from './components/Input';
import Messages from './components/Messages';
import styled from 'styled-components';



const App = () => {
    const [isChatVisible, setIsChatVisible] = useState(false);
    const [chatID, setChatID] = useState(null);
    const {isLoading, isAuthenticated} = useAuth0();

    const toggleChatVisibility = () => {
        setIsChatVisible(!isChatVisible);
    };



    if (isLoading) return <div>Loading...</div>;
    // if (!isAuthenticated) return <div>Please log in to use the app.</div>;

    return (
        <StyledApp>
            <Background/>
            <Header toggleChatVisibility={toggleChatVisibility} isChatVisible={isChatVisible}/>
            <MainContent>
                <ChatContainer isChatVisible={isChatVisible} setChatID={setChatID}/>
                <MessagesContainer isChatVisible={isChatVisible}>
                    <Messages chatID={chatID}/>
                    <InputContainer>
                        <Input chatID={chatID}/>
                    </InputContainer>
                </MessagesContainer>
            </MainContent>
        </StyledApp>
    );
};

const StyledApp = styled.div`
    display: flex;
    flex-direction: column;
    height: 100vh;
    width: 100vw;
`;

const MainContent = styled.div`
    display: flex;
    flex: 1;
    overflow: hidden;
    padding-top: 60px;
`;

const MessagesContainer = styled.div`
    display: flex;
    flex-direction: column;
    width: ${({isChatVisible}) => (isChatVisible ? 'calc(100vw - 20px)' : 'calc(100vw - 460px)')};
    margin-left: auto;
    overflow: hidden;
    transition: flex 0.3s ease, width 0.3s ease;
    justify-content: flex-end;

    @media (max-width: 768px) {
        width: calc(100vw - 20px);
    }
`;

const InputContainer = styled.div`
    flex-shrink: 1;
    display: flex;
    justify-content: flex-end;
`;

export default App;