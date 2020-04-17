package main

import (
	"net/http"
	"time"

	"github.com/gorilla/mux"
	"github.com/julienschmidt/sse"
)

type ctevent struct {
	Id         string `json:"id"`
	ContractId string `json:"contract_id,omitempty"`
	Message    string `json:"message,omitempty"`
	Kind       string `json:"kind,omitempty"`
}

func dispatchContractEvent(contractId string, ev ctevent, typ string) {
	if ies, ok := contractstreams.Get(contractId); ok {
		ies.(*sse.Streamer).SendJSON("", typ, ev)
	}
}

func contractStream(w http.ResponseWriter, r *http.Request) {
	ctid := mux.Vars(r)["ctid"]

	var es *sse.Streamer
	ies, ok := contractstreams.Get(ctid)

	if !ok {
		es = sse.New()

		go func() {
			for {
				time.Sleep(25 * time.Second)
				es.SendString("", "keepalive", "")
			}
		}()
		contractstreams.Set(ctid, es)
	} else {
		es = ies.(*sse.Streamer)
	}

	es.ServeHTTP(w, r)
}

type callPrinter struct {
	ContractId string
	CallId     string
}

func (cp *callPrinter) Write(data []byte) (n int, err error) {
	dispatchContractEvent(cp.ContractId, ctevent{cp.CallId, cp.ContractId, string(data), "print"}, "call-run-event")
	return len(data), nil
}
