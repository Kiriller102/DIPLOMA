import { Auth0Provider } from "@auth0/auth0-react";

const AuthProvider = ({ children }) => {
    return (
        <Auth0Provider
            domain="dev-nj0jj2ipl86hwk30.us.auth0.com"
            clientId="DOV2VUejSf1ctQDIjasntzwYVwgjTQTW"
            authorizationParams={{
                redirect_uri: window.location.origin
            }}
        >
            {children}
        </Auth0Provider>
    );
};

export default AuthProvider;
