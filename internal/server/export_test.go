package server

// WithLocalOrigin exposes withLocalOrigin so tests can send it requests from
// addresses a real listener on loopback never sees.
var WithLocalOrigin = withLocalOrigin
