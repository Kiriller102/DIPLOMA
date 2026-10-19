import { useState, useEffect } from 'react';
import styled from 'styled-components';
import { api } from '../api';

const ChatContainer = ({ isChatVisible, setChatID, chatID, userID, users }) => {
    const [chats, setChats] = useState([]);
    const [name, setName] = useState('');
    const [memberID, setMemberID] = useState('');
    const [error, setError] = useState('');
    const [pending, setPending] = useState(false);

    useEffect(() => {
        const controller = new AbortController();
        let timer;
        setChats([]);
        const refresh = async () => {
            try {
                const data = await api(`/chat_participant/chats/${userID}`, { signal: controller.signal });
                if (!controller.signal.aborted) { setChats(data); setError(''); }
            } catch (err) {
                if (err.name !== 'AbortError') setError(err.message);
            } finally {
                if (!controller.signal.aborted) timer = setTimeout(refresh, 3000);
            }
        };
        refresh();
        return () => { controller.abort(); clearTimeout(timer); };
    }, [userID]);

    const createChat = async (event) => {
        event.preventDefault();
        if (!name.trim() || pending) return;
        setPending(true);
        try {
            const { chat } = await api('/chat', {
                method: 'POST', body: JSON.stringify({ name: name.trim(), type: 'group', created_by: userID,
                    participant_ids: memberID ? [Number(memberID)] : [] }),
            });
            setChats(current => [...current.filter(item => item.id !== chat.id), chat]);
            setChatID(chat.id);
            setName('');
            setError('');
        } catch (err) { setError(err.message); }
        finally { setPending(false); }
    };

    return <StyledChatBox $hidden={isChatVisible}>
        <h3>Мои чаты</h3>
        <form onSubmit={createChat}>
            <input aria-label="Название чата" placeholder="Название нового чата" maxLength={100} value={name} onChange={event => setName(event.target.value)} required />
            <select aria-label="Участник нового чата" value={memberID} onChange={event => setMemberID(event.target.value)}>
                <option value="">Только я</option>
                {users.filter(user => user.id !== userID).map(user => <option key={user.id} value={user.id}>{user.first_name || user.username}</option>)}
            </select>
            <button disabled={pending || !name.trim()}>{pending ? 'Создание…' : 'Создать чат'}</button>
        </form>
        {error && <p role="alert">{error}</p>}
        {!chats.length && <p>Создайте чат или дождитесь приглашения.</p>}
        {chats.map(chat => <button className={chatID === chat.id ? 'chat selected' : 'chat'} key={chat.id} onClick={() => setChatID(chat.id)}>{chat.name}</button>)}
    </StyledChatBox>;
};

const StyledChatBox = styled.aside`
    display: ${({ $hidden }) => $hidden ? 'none' : 'block'};
    width: min(360px, 45vw);
    flex-shrink: 0;
    box-sizing: border-box;
    background: #f0f0f0;
    color: #20212a;
    border-radius: 15px;
    padding: 16px;
    margin: 10px;
    overflow-y: auto;
    form { display: grid; gap: 8px; margin-bottom: 16px; }
    input, select { min-width: 0; width: 100%; box-sizing: border-box; padding: 8px; }
    .chat { display: block; width: 100%; margin-bottom: 8px; text-align: left; overflow-wrap: anywhere; }
    .selected { border-color: #7079f0; }
`;
export default ChatContainer;
