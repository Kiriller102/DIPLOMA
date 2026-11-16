import React from 'react';
import styled from 'styled-components';


const Header = ({ toggleChatVisibility, isChatVisible, user, logout, signingOut }) => {
    return (
        <StyledHeader>
            <div>
                <input id="checkbox" type="checkbox" onChange={toggleChatVisibility} checked={isChatVisible} />
                <label className="toggle" htmlFor="checkbox">
                    <div id="bar1" className="bars"/>
                    <div id="bar2" className="bars"/>
                    <div id="bar3" className="bars"/>
                </label>
            </div>
            <h2>Messenger</h2>
            <div className="account">
                <span aria-label="Текущий пользователь">{user.first_name || user.username}</span>
                <button onClick={logout} disabled={signingOut}>{signingOut ? 'Выход…' : 'Выйти'}</button>
            </div>
        </StyledHeader>
    );
}


const StyledHeader = styled.header`
    position: fixed;
    top: 0;
    left: 0;
    width: 100vw;
    height: 60px;
    background-color: rgba(52, 51, 51);
    color: white;
    display: flex;
    align-items: center;

    font-size: 20px;
    z-index: 100;

    h2 { margin: 0; font-size: 22px; }
    .account { margin-left: auto; margin-right: 12px; display: flex; align-items: center; gap: 10px; min-width: 0; font-size: 14px; }
    .account span { max-width: 180px; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
    @media (max-width: 600px) {
        .account span { max-width: 80px; }
        h2 { font-size: 18px; }
    }

    #checkbox {
        display: none;
    }

    .toggle {
        position: relative;
        width: 40px;
        height: 40px;
        cursor: pointer;
        display: flex;
        flex-direction: column;
        align-items: center;
        justify-content: center;
        margin: 10px;
        gap: 10px;
        transition-duration: .3s;
    }

    .bars {
        width: 100%;
        height: 4px;
        background-color: rgb(253, 255, 243);
        border-radius: 5px;
        transition-duration: .3s;
    }


    #checkbox:checked + .toggle #bar2 {
        transform: translateY(14px) rotate(60deg);
        margin-left: 0;
        transform-origin: right;
        transition-duration: .3s;
        z-index: 2;
    }

    #checkbox:checked + .toggle #bar1 {
        transform: translateY(28px) rotate(-60deg);
        transition-duration: .3s;
        transform-origin: left;
        z-index: 1;
    }

    #checkbox:checked + .toggle {
        transform: rotate(-90deg);
    }
`;

export default Header;