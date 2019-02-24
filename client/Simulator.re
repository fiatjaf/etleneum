open Rebase;

type state = {
  contract: API.contract,
  temp_contract_state: option(string),
  nextcall: API.call,
  temp_call_payload: option(string),
  result: option(luaresult),
  available_methods: list(string),
}
and luaresult = {
  state: Js.Json.t,
  ret: Js.Json.t,
  payments_done: list(string),
  total_paid: int,
  error: string,
};

type action =
  | EditContractCode(string)
  | EditContractState(string)
  | ParseContractStateJSON
  | EditContractFunds(int)
  | EditCallMethod(string)
  | EditCallPayload(string)
  | ParseCallPayloadJSON
  | EditCallSatoshis(int)
  | SetState(state)
  | SimulateCall;

[@bs.module "./glua-simulate-environment.js"]
external runlua: (string, Js.Json.t, string, Js.Json.t, int) => Js.Json.t =
  "runlua";

let getluaresult = (json: Js.Json.t): luaresult =>
  Json.Decode.{
    state: json |> field("stateAfter", x => x),
    ret: json |> field("returnedValue", x => x),
    payments_done: json |> field("paymentsDone", list(string)),
    total_paid: json |> field("totalPaid", int),
    error: json |> field("error", string),
  };

let component = ReasonReact.reducerComponent("Simulator");

let make = (~preloadContract=?, ~preloadCall=?, _children) => {
  ...component,
  initialState: () => {
    let contract =
      switch (preloadContract) {
      | None =>
        switch (API.LS.getItem("simulating-contract")) {
        | None => API.emptyContract
        | Some(jstr) => jstr |> Js.Json.parseExn |> API.Decode.contract
        }
      | Some(precon) => precon
      };

    {
      contract,
      nextcall:
        switch (preloadContract, preloadCall) {
        | (None, None) =>
          switch (API.LS.getItem("simulating-call")) {
          | None => API.emptyCall
          | Some(jstr) => jstr |> Js.Json.parseExn |> API.Decode.call
          }
        | (Some(_), None) => API.emptyCall
        | (None, Some(precall))
        | (Some(_), Some(precall)) => precall
        },
      temp_call_payload: None,
      temp_contract_state: None,
      result: None,
      available_methods: API.Helpers.parseMethods(contract.code),
    };
  },
  reducer: (action: action, state: state) => {
    let contract = state.contract;
    let nextcall = state.nextcall;

    switch (action) {
    | EditContractCode(code) =>
      let available_methods = API.Helpers.parseMethods(code);
      let method =
        if (available_methods |> List.exists(m => m == state.nextcall.method)) {
          state.nextcall.method;
        } else {
          switch (List.head(available_methods)) {
          | None => ""
          | Some(method) => method
          };
        };

      ReasonReact.SideEffects(
        self =>
          self.send(
            SetState({
              ...state,
              contract: {
                ...contract,
                code,
              },
              available_methods,
              nextcall: {
                ...nextcall,
                method,
              },
            }),
          ),
      );
    | EditContractState(statestr) =>
      ReasonReact.Update({...state, temp_contract_state: Some(statestr)})
    | ParseContractStateJSON =>
      ReasonReact.SideEffects(
        self =>
          switch (self.state.temp_contract_state) {
          | None => ()
          | Some(temp) =>
            switch (Js.Json.parseExn(temp)) {
            | json =>
              {
                ...state,
                contract: {
                  ...contract,
                  state: json,
                },
                temp_contract_state: None,
              }
              ->SetState
              |> self.send
            | exception (Js.Exn.Error(_)) => ()
            }
          },
      )
    | EditContractFunds(funds) =>
      ReasonReact.SideEffects(
        self =>
          self.send(SetState({
                      ...state,
                      contract: {
                        ...contract,
                        funds,
                      },
                    })),
      )
    | EditCallMethod(method) =>
      ReasonReact.SideEffects(
        self =>
          self.send(SetState({
                      ...state,
                      nextcall: {
                        ...nextcall,
                        method,
                      },
                    })),
      )
    | EditCallPayload(payload) =>
      ReasonReact.Update({...state, temp_call_payload: Some(payload)})
    | ParseCallPayloadJSON =>
      ReasonReact.SideEffects(
        self =>
          switch (self.state.temp_call_payload) {
          | None => ()
          | Some(temp) =>
            switch (Js.Json.parseExn(temp)) {
            | json =>
              {
                ...state,
                nextcall: {
                  ...nextcall,
                  payload: json,
                },
                temp_call_payload: None,
              }
              ->SetState
              |> self.send
            | exception (Js.Exn.Error(_)) => ()
            }
          },
      )
    | EditCallSatoshis(satoshis) =>
      ReasonReact.SideEffects(
        self =>
          self.send(
            SetState({
              ...state,
              nextcall: {
                ...nextcall,
                satoshis,
              },
            }),
          ),
      )
    | SetState(state) =>
      ReasonReact.UpdateWithSideEffects(
        state,
        _self => {
          API.LS.setItem(
            "simulating-contract",
            API.Encode.contract(state.contract),
          );
          API.LS.setItem("simulating-call", API.Encode.call(state.nextcall));
        },
      )
    | SimulateCall =>
      ReasonReact.Update({
        ...state,
        result:
          runlua(
            state.contract.code,
            state.contract.state,
            state.nextcall.method,
            state.nextcall.payload,
            state.nextcall.satoshis,
          )
          ->getluaresult
          ->Some,
      })
    };
  },
  render: self =>
    <div className="simulator">
      <div> <h1> {ReasonReact.string("Simulator")} </h1> </div>
      <div className="elements">
        <div className="code">
          {ReasonReact.string("Lua code: ")}
          <div>
            <textarea
              onChange={
                self.handle((event, _self) =>
                  self.send(
                    EditContractCode(event->ReactEvent.Form.target##value),
                  )
                )
              }
              value={self.state.contract.code}
            />
          </div>
        </div>
        <div className="state">
          <div> {ReasonReact.string("Contract state:")} </div>
          <textarea
            onChange={
              self.handle((event, _self) =>
                self.send(
                  EditContractState(event->ReactEvent.Form.target##value),
                )
              )
            }
            onBlur={
              self.handle((_event, _self) =>
                self.send(ParseContractStateJSON)
              )
            }
            value={
              switch (self.state.temp_contract_state) {
              | None =>
                Js.Json.stringifyWithSpace(self.state.contract.state, 2)
              | Some(str) => str
              }
            }
          />
        </div>
      </div>
      <div className="elements">
        <div className="nextcall">
          <div> <h3> {ReasonReact.string("Next call:")} </h3> </div>
          <div>
            <h5> {ReasonReact.string("Method: ")} </h5>
            <select
              value={self.state.nextcall.method}
              onChange={
                self.handle((event, _self) =>
                  self.send(
                    EditCallMethod(event->ReactEvent.Form.target##value),
                  )
                )
              }>
              {ReasonReact.array(
                 Array.fromList(
                   self.state.available_methods
                   |> List.concat(["__init__"])
                   |> List.map(m =>
                        <option key=m> {ReasonReact.string(m)} </option>
                      ),
                 ),
               )}
            </select>
          </div>
          <div>
            <h5> {ReasonReact.string("Satoshis: ")} </h5>
            <input
              type_="number"
              step=1.0
              min=0
              value={string_of_int(self.state.nextcall.satoshis)}
              onChange={
                self.handle((event, _self) =>
                  self.send(
                    EditCallSatoshis(
                      event->ReactEvent.Form.target##value |> int_of_string,
                    ),
                  )
                )
              }
            />
          </div>
          <div>
            <h5> {ReasonReact.string("Payload: ")} </h5>
            <textarea
              onChange={
                self.handle((event, _self) =>
                  self.send(
                    EditCallPayload(event->ReactEvent.Form.target##value),
                  )
                )
              }
              onBlur={
                self.handle((_event, _self) =>
                  self.send(ParseCallPayloadJSON)
                )
              }
              value={
                switch (self.state.temp_call_payload) {
                | None =>
                  Js.Json.stringifyWithSpace(self.state.nextcall.payload, 2)
                | Some(str) => str
                }
              }
            />
          </div>
          <div className="button">
            <button
              onClick={
                self.handle((_event, _self) => self.send(SimulateCall))
              }>
              {ReasonReact.string("Simulate call")}
            </button>
          </div>
        </div>
        <div>
          <div> <h3> {ReasonReact.string("Result:")} </h3> </div>
          {switch (self.state.result) {
           | None => <div />
           | Some(result) =>
             if (result.error == "") {
               <div className="result">
                 <div>
                   <label> {ReasonReact.string("State: ")} </label>
                   <ReactJSONView
                     src={result.state}
                     name="state"
                     theme="summerfruit-inverted"
                     iconStyle="triangle"
                     indentWidth=2
                     collapsed=2
                     enableClipboard=false
                     displayDataTypes=false
                     sortKeys=true
                   />
                   <button
                     onClick={
                               let state = self.state;
                               let contract = self.state.contract;
                               self.handle((_event, self) =>
                                 self.send(
                                   SetState({
                                     ...state,
                                     contract: {
                                       ...contract,
                                       state: result.state,
                                     },
                                     temp_contract_state: None,
                                   }),
                                 )
                               );
                             }>
                     {ReasonReact.string("Apply this")}
                   </button>
                 </div>
                 <div>
                   <label> {ReasonReact.string("Returned value: ")} </label>
                   <ReactJSONView
                     src={result.ret}
                     name="ret"
                     theme="summerfruit-inverted"
                     iconStyle="triangle"
                     indentWidth=2
                     collapsed=2
                     enableClipboard=false
                     displayDataTypes=false
                     sortKeys=true
                   />
                 </div>
                 <div>
                   {if (result.payments_done |> List.length == 0) {
                      <label>
                        {ReasonReact.string("No payments made.")}
                      </label>;
                    } else {
                      <div>
                        <label>
                          {ReasonReact.string(
                             "Payments made ("
                             ++ string_of_int(result.total_paid)
                             ++ " msats): ",
                           )}
                        </label>
                        <ul>
                          {ReasonReact.array(
                             Array.fromList(
                               result.payments_done
                               |> List.map(bolt11 =>
                                    <li key=bolt11>
                                      {ReasonReact.string(bolt11)}
                                    </li>
                                  ),
                             ),
                           )}
                        </ul>
                      </div>;
                    }}
                 </div>
               </div>;
             } else {
               <pre> {ReasonReact.string(result.error)} </pre>;
             }
           }}
        </div>
      </div>
    </div>,
};
