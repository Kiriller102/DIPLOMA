import { useState } from 'react';
import styled from 'styled-components';
import { api } from '../api';

export default function AuthForm({ onAuthenticated, connectionError, retry }) {
    const [register, setRegister] = useState(false);
    const [username, setUsername] = useState('');
    const [email, setEmail] = useState('');
    const [password, setPassword] = useState('');
    const [error, setError] = useState('');
    const [pending, setPending] = useState(false);
    const submit = async event => {
        event.preventDefault();
        if (pending) return;
        if (register && Array.from(password).length < 12) { setError('Пароль должен содержать минимум 12 символов.'); return; }
        if (new TextEncoder().encode(password).length > 72) { setError('Пароль слишком длинный.'); return; }
        setPending(true);
        setError('');
        try {
            const { user } = await api(register ? '/auth/register' : '/auth/login', {
                method: 'POST', body: JSON.stringify(register ? { username, email, password } : { email, password }),
            });
            setPassword('');
            onAuthenticated(user);
        } catch (err) {
            if (err.status === 401) setError('Неверный email или пароль.');
            else if (err.status === 409) setError('Этот email или имя пользователя уже заняты.');
            else if (err.status === 429) setError('Слишком много попыток. Попробуйте через минуту.');
            else if (err.status === 400) setError('Проверьте email, имя пользователя и длину пароля.');
            else setError('Не удалось связаться с сервером. Попробуйте ещё раз.');
        } finally { setPending(false); }
    };
    return <Page><Panel>
        <h1>Messenger</h1>
        <h2>{register ? 'Создать аккаунт' : 'Вход'}</h2>
        {connectionError && <p role="alert">Сервер недоступен. <button onClick={retry}>Повторить</button></p>}
        <form onSubmit={submit}>
            {register && <label>Имя пользователя<input aria-label="Имя пользователя" aria-describedby="username-hint" autoComplete="username" value={username} onChange={event => setUsername(event.target.value)} minLength={3} maxLength={50} required disabled={pending} /><small id="username-hint">От 3 до 50 букв, цифр или символов _ . -</small></label>}
            <label>Email<input type="email" autoComplete={register ? 'email' : 'username'} value={email} onChange={event => setEmail(event.target.value)} maxLength={100} required disabled={pending} /></label>
            <label>Пароль<input aria-label="Пароль" aria-describedby={register ? "password-hint" : undefined} type="password" autoComplete={register ? 'new-password' : 'current-password'} value={password} onChange={event => setPassword(event.target.value)} minLength={register ? 12 : 1} maxLength={72} required disabled={pending} />{register && <small id="password-hint">Минимум 12 символов.</small>}</label>
            {error && <p role="alert">{error}</p>}
            <button type="submit" disabled={pending}>{pending ? 'Подождите…' : register ? 'Зарегистрироваться' : 'Войти'}</button>
        </form>
        <button className="switch" disabled={pending} onClick={() => { setRegister(value => !value); setError(''); setPassword(''); }}>{register ? 'Уже есть аккаунт? Войти' : 'Создать аккаунт'}</button>
    </Panel></Page>;
}

const Page = styled.main`
    min-height: 100dvh; width: 100%; display: grid; place-items: center; padding: 24px; box-sizing: border-box;
    background: #20212a; color: #f2f2f5;
`;
const Panel = styled.section`
    width: min(100%, 380px); background: #2c2d38; border: 1px solid #414252; border-radius: 18px; padding: 28px; box-sizing: border-box;
    h1 { font-size: 26px; margin: 0 0 8px; } h2 { font-size: 20px; margin: 0 0 24px; }
    form, label { display: grid; gap: 8px; } form { gap: 18px; }
    input { min-width: 0; padding: 10px; border: 1px solid #666879; border-radius: 8px; background: #20212a; color: #fff; font: inherit; }
    small { color: #b5b7c9; } p { margin: 0; } .switch { margin-top: 18px; width: 100%; background: transparent; }
    button[type=submit] { background: #7079f0; color: white; }
`;
