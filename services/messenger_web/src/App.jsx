import { useEffect, useState } from 'react';
import Header from './components/Header';
import { api } from './api';
import Background from './components/Background';
import ChatContainer from './components/Chats';
import Input from './components/Input';
import Messages from './components/Messages';
import styled from 'styled-components';



const App = () => {
    const [isChatVisible, setIsChatVisible] = useState(false);
    const [chatID, setChatID] = useState(null);
    const [users, setUsers] = useState([]);
    const [userID, setUserID] = useState(null);
    const [error, setError] = useState('');
    const [loading, setLoading] = useState(true);
    const [reload, setReload] = useState(0);
    useEffect(() => {
        const controller = new AbortController();
        setLoading(true);
        api('/users/all', { signal: controller.signal }).then(data => {
            setUsers(data);
            const saved = Number(sessionStorage.getItem('messenger-user'));
            setUserID(data.some(user => user.id === saved) ? saved : (data[0]?.id ?? null));
            setError('');
        }).catch(err => { if (err.name !== 'AbortError') setError(err.message); })
          .finally(() => { if (!controller.signal.aborted) setLoading(false); });
        return () => controller.abort();
    }, [reload]);
    const selectUser = (id) => {
        setUserID(id);
        setChatID(null);
        sessionStorage.setItem('messenger-user', String(id));
    };

    const toggleChatVisibility = () => {
        setIsChatVisible(!isChatVisible);
    };



    if (loading) return <p role="status">Загрузка пользователей…</p>;
    if (error) return <div role="alert">{error} <button onClick={() => setReload(value => value + 1)}>Повторить</button></div>;
    if (!users.length) return <p>Пользователей пока нет. Включите DEMO_MODE=true или создайте пользователя через API.</p>;

    return (
        <StyledApp>
            <Background/>
            <Header toggleChatVisibility={toggleChatVisibility} isChatVisible={isChatVisible} users={users} userID={userID} selectUser={selectUser}/>
            <MainContent>
                <ChatContainer isChatVisible={isChatVisible} setChatID={setChatID} chatID={chatID} userID={userID} users={users}/>
                <MessagesContainer>
                    <Messages key={`${userID}-${chatID}`} chatID={chatID} userID={userID} users={users}/>
                    <InputContainer>
                        <Input key={`${userID}-${chatID}`} chatID={chatID} userID={userID}/>
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
    flex: 1;
    min-width: 0;
    margin: 10px;
    margin-left: auto;
    overflow: hidden;
    transition: flex 0.3s ease, width 0.3s ease;
    justify-content: flex-end;

`;

const InputContainer = styled.div`
    flex-shrink: 1;
    display: flex;
    justify-content: flex-end;
`;

export default App;