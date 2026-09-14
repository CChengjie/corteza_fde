// Keep browser API and auth requests on the client origin so the dev-server
// proxy can preserve the session during Cypress runs.
window.CortezaAPI = '/api'
window.CortezaProxyURL = 'http://127.0.0.1:8888'

window.i18nPseudoModeEnabled = false
