import React from "react";
import { useAuth0 } from "@auth0/auth0-react";

const AuthButtons = () => {
    const { loginWithRedirect, logout, isAuthenticated, user } = useAuth0();  

    return (
        <div style={{ marginLeft: 'auto', padding: '10px' }}>
            {isAuthenticated ? (
                <>
                    {/*<p>Привет, {user.name}!</p>*/}
                    <button onClick={() => logout({ returnTo: window.location.origin })}>
                        Выйти
                    </button>
                </>
            ) : (
                <button onClick={() => loginWithRedirect()}>Войти</button>
            )}
        </div>
    );
};

export default AuthButtons;
