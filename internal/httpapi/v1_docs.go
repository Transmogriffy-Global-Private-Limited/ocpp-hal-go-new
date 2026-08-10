package httpapi

import (
	_ "embed"
	"net/http"
)

//go:embed v1_openapi.json
var v1OpenAPI []byte

func (s *Server) registerAPIDocs(mux *http.ServeMux) {
	mux.HandleFunc("/openapi.json", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(v1OpenAPI)
	})
	mux.HandleFunc("/docs", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write([]byte(v1DocsHTML))
	})
}

const v1DocsHTML = `<!doctype html><html><head><meta charset="utf-8"><title>OCPP HAL API</title><style>body{font-family:system-ui;margin:2rem;max-width:70rem}input,textarea,select,button{font:inherit;margin:.25rem;padding:.4rem}textarea{width:100%;height:12rem}pre{background:#f3f5f7;padding:1rem;white-space:pre-wrap}</style></head><body><h1>OCPP HAL API</h1><p>Machine-readable contract: <a href="/openapi.json">/openapi.json</a></p><p>This loopback-friendly explorer sends only the request you enter. Use the opaque service bearer token; never place it in the OpenAPI document.</p><label>Method <select id="method"><option>GET</option><option>POST</option><option>PUT</option></select></label><label>Path <input id="path" size="55" value="/v1/remote-commands?cms_command_id="></label><br><label>Bearer token <input id="token" type="password" size="55"></label><br><label>Idempotency-Key <input id="idempotency" size="40"></label><label>X-Correlation-ID <input id="correlation" size="40"></label><br><label>JSON body</label><textarea id="body">{}</textarea><br><button id="send">Send request</button><pre id="result"></pre><script>document.getElementById('send').onclick=async()=>{const h={Authorization:'Bearer '+token.value};if(idempotency.value)h['Idempotency-Key']=idempotency.value;if(correlation.value)h['X-Correlation-ID']=correlation.value;let o={method:method.value,headers:h};if(method.value!=='GET'){h['Content-Type']='application/json';o.body=body.value}try{let r=await fetch(path.value,o);result.textContent=r.status+' '+await r.text()}catch(e){result.textContent=String(e)}};</script></body></html>`
