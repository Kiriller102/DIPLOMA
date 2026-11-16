import { useEffect, useState } from 'react';
import Header from './components/Header';
import AuthForm from './components/AuthForm';
import { api } from './api';
import Background from './components/Background';
import ChatContainer from './components/Chats';
import Input from './components/Input';
import Messages from './components/Messages';
import EncryptionUnlock from './components/EncryptionUnlock';
import styled from 'styled-components';



const App = () => {
    const [isChatVisible, setIsChatVisible] = useState(false);
    const [chatID, setChatID] = useState(null);
    const [users, setUsers] = useState([]);
    const [user, setUser] = useState(null);
    const [error, setError] = useState('');
    const [loading, setLoading] = useState(true);
    const [reload, setReload] = useState(0);
    const [signingOut, setSigningOut] = useState(false);
    const [cryptoSession, setCryptoSession] = useState(null);
    const [algorithm, setAlgorithm] = useState('AES');
    useEffect(() => {
        const controller = new AbortController();
        setLoading(true);
        const load = async () => {
            try {
                const { user: current } = await api('/auth/me', { signal: controller.signal });
                const directory = await api('/users/all', { signal: controller.signal });
                if (!controller.signal.aborted) { setUser(current); setUsers(directory); setError(''); }
            } catch (err) {
                if (err.name === 'AbortError') return;
                if (err.status === 401) { setUser(null); setUsers([]); setChatID(null); setError(''); }
                else setError(err.message);
            } finally { if (!controller.signal.aborted) setLoading(false); }
        };
        load();
        return () => controller.abort();
    }, [reload]);
    useEffect(() => {
        const expired = () => { setUser(null); setUsers([]); setChatID(null); setCryptoSession(null); };
        const sync = event => { if (event.key === 'messenger-auth-change') { setChatID(null); setCryptoSession(null); setReload(value => value + 1); } };
        window.addEventListener('messenger:unauthenticated', expired);
        window.addEventListener('storage', sync);
        return () => { window.removeEventListener('messenger:unauthenticated', expired); window.removeEventListener('storage', sync); };
    }, []);
    const notifyTabs = () => { try { localStorage.setItem('messenger-auth-change', String(Date.now())); } catch { /* Storage may be unavailable; the server still checks every request. */ } };
    const authenticated = current => { setLoading(true); setIsChatVisible(false); setUser(current); setChatID(null); setCryptoSession(null); setReload(value => value + 1); notifyTabs(); };
    const logout = async () => {
        if (signingOut) return;
        setSigningOut(true);
        try { await api('/auth/logout', { method: 'POST' }); setCryptoSession(null); setUser(null); setUsers([]); setChatID(null); setError(''); setReload(value => value + 1); notifyTabs(); }
        catch (err) { setError(`Не удалось выйти: ${err.message}`); }
        finally { setSigningOut(false); }
    };
    if (loading) return <p role="status">Загрузка…</p>;
    if (!user) return <AuthForm onAuthenticated={authenticated} connectionError={error} retry={() => setReload(value => value + 1)} />;
    if (cryptoSession?.userID !== user.id) return <EncryptionUnlock key={user.id} user={user} onUnlocked={setCryptoSession} logout={logout} />;
    return <StyledApp>
        <Background />
        <Header toggleChatVisibility={() => setIsChatVisible(value => !value)} isChatVisible={isChatVisible} user={user} logout={logout} signingOut={signingOut} />
        {error && <div role="alert" style={{ position: 'fixed', top: 60, zIndex: 110 }}>{error} <button onClick={() => setReload(value => value + 1)}>Повторить</button></div>}
        <MainContent>
            <ChatContainer key={user.id} isChatVisible={isChatVisible} setChatID={setChatID} chatID={chatID} userID={user.id} users={users} />
            <MessagesContainer>
                <Messages key={`${user.id}-${chatID}`} chatID={chatID} userID={user.id} users={users} cryptoSession={cryptoSession} algorithm={algorithm} />
                <InputContainer><Input key={`${user.id}-${chatID}`} chatID={chatID} userID={user.id} cryptoSession={cryptoSession} algorithm={algorithm} setAlgorithm={setAlgorithm} /></InputContainer>
            </MessagesContainer>
        </MainContent>
    </StyledApp>;
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
