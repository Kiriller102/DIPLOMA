import React from 'react';
import styled from 'styled-components';
import AuthButtons from "./AuthButtons.jsx";

const Header = ({ toggleChatVisibility, isChatVisible }) => {
    return (
        <StyledHeader>
            <div>
                <input id="checkbox" type="checkbox" onClick={toggleChatVisibility} checked={isChatVisible} />
                <label className="toggle" htmlFor="checkbox">
                    <div id="bar1" className="bars"/>
                    <div id="bar2" className="bars"/>
                    <div id="bar3" className="bars"/>
                </label>
            </div>
            <h2>Messenger</h2>
            <AuthButtons />
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
    //justify-content: center;
    font-size: 20px;
    z-index: 100;

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

const ToggleButton = styled.button`
    padding: 10px 20px;
    margin: 10px;
    background-color: #fff;
    color: #007bff;
    border: none;
    border-radius: 5px;
    cursor: pointer;
`;

export default Header;