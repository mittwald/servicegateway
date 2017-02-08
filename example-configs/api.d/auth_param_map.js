var exports = function (username, password) {
    var body = {ttl: 86400};
    var url = 'https://identity.service.consul/authenticate';

    if (username == "" && password == "") {
        return false;
    }
    
    body.providers = ["public"];
    body.username = username;
    body.password = password;

    return {
        body: body,
        url: url
    };
};