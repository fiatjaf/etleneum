[@bs.module "./markdown.js"] external markdown: string => string = "markdown";

type state = {
  calls: list(API.call),
  callopen: option(string),
  nextcall: API.call,
  temp_call_payload: option(string),
};

type action =
  | LoadCalls
  | GotCalls(list(API.call))
  | OpenCall(string)
  | CloseCall
  | EditCallMethod(string)
  | EditCallPayload(string)
  | ParseCallPayloadJSON
  | EditCallSatoshis(int)
  | SetState(state)
  | PrepareCall
  | CallPrepared(API.call);

let betterdate = datestr =>
  try (String.sub(datestr, 0, 10) ++ " at " ++ String.sub(datestr, 11, 8)) {
  | Invalid_argument(_) => datestr
  };

let component = ReasonReact.reducerComponent("ContractPage");

let make = (~contract: API.contract, _children) => {
  ...component,
  didMount: self => self.send(LoadCalls),
  initialState: () => {
    calls: [],
    callopen: None,
    nextcall:
      switch (API.LS.getItem("next-call:" ++ contract.id)) {
      | None => API.emptyCall
      | Some(jstr) => jstr |> Js.Json.parseExn |> API.Decode.call
      },
    temp_call_payload: None,
  },
  reducer: (action: action, state: state) => {
    let nextcall = state.nextcall;

    switch (action) {
    | LoadCalls =>
      ReasonReact.SideEffects(
        self => {
          let _ =
            API.Call.list(contract.id)
            |> Js.Promise.then_(v =>
                 self.send(GotCalls(v)) |> Js.Promise.resolve
               );
          ();
        },
      )
    | GotCalls(calls) => ReasonReact.Update({...state, calls})
    | OpenCall(callid) =>
      ReasonReact.Update({...state, callopen: Some(callid)})
    | CloseCall => ReasonReact.Update({...state, callopen: None})
    | EditCallMethod(method) =>
      ReasonReact.Update({
        ...state,
        nextcall: {
          ...nextcall,
          method,
        },
      })
    | EditCallPayload(jstr) =>
      ReasonReact.Update({...state, temp_call_payload: Some(jstr)})
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
      ReasonReact.Update({
        ...state,
        nextcall: {
          ...nextcall,
          satoshis,
        },
      })
    | SetState(state) =>
      ReasonReact.UpdateWithSideEffects(
        state,
        _self =>
          API.LS.setItem(
            "next-call:" ++ contract.id,
            API.Encode.call(state.nextcall),
          ),
      )
    | PrepareCall =>
      ReasonReact.SideEffects(
        self => {
          let _ =
            API.Call.prepare(contract.id, self.state.nextcall)
            |> Js.Promise.then_(v =>
                 self.send(CallPrepared(v)) |> Js.Promise.resolve
               );
          ();
        },
      )
    | CallPrepared(call) =>
      ReasonReact.SideEffects(
        _self => ReasonReact.Router.push("/call/" ++ call.id),
      )
    };
  },
  render: self =>
    <div className="contract">
      <div>
        <header>
          <h1> {ReasonReact.string(contract.name)} </h1>
          <aside>
            {ReasonReact.string("since ")}
            <time dateTime={contract.created_at}>
              {ReasonReact.string(contract.created_at |> betterdate)}
            </time>
          </aside>
        </header>
        <div className="simulate">
          <a
            onClick={
              self.handle((_event, _self) =>
                ReasonReact.Router.push("/simulator/" ++ contract.id)
              )
            }>
            {ReasonReact.string("Try on simulator")}
          </a>
        </div>
      </div>
      <div>
        <div className="readme">
          <h3> {ReasonReact.string("Readme")} </h3>
          <article
            dangerouslySetInnerHTML={"__html": markdown(contract.readme)}
          />
        </div>
        <div>
          <h3> {ReasonReact.string("Code")} </h3>
          <textarea readOnly=true value={contract.code} />
        </div>
      </div>
      <div>
        <div className="calls">
          <h3> {ReasonReact.string("Latest calls")} </h3>
          {if (self.state.calls |> List.length == 0) {
             <p>
               {ReasonReact.string("No calls were made to this contract yet.")}
             </p>;
           } else {
             ReasonReact.array(
               Array.of_list(
                 self.state.calls
                 |> List.map((call: API.call) =>
                      <div
                        key={call.id}
                        className={
                          "call-item"
                          ++ (
                            if (self.state.callopen == Some(call.id)) {
                              " open";
                            } else {
                              "";
                            }
                          )
                        }>
                        <div>
                          <a
                            className="highlight"
                            onClick={
                              self.handle((_event, self) =>
                                if (self.state.callopen == Some(call.id)) {
                                  self.send(CloseCall);
                                } else {
                                  self.send(OpenCall(call.id));
                                }
                              )
                            }>
                            {ReasonReact.string(call.id)}
                          </a>
                        </div>
                        <div> {ReasonReact.string(call.method)} </div>
                        <div>
                          {ReasonReact.string(call.time |> betterdate)}
                        </div>
                        <div className="body">
                          {if (self.state.callopen == Some(call.id)) {
                             <div>
                               <div>
                                 <ReactJSONView
                                   src={call.payload}
                                   name="payload"
                                   theme="summerfruit-inverted"
                                   iconStyle="triangle"
                                   indentWidth=2
                                   collapsed=2
                                   enableClipboard=false
                                   displayDataTypes=false
                                   sortKeys=true
                                 />
                               </div>
                               <div className="desc">
                                 {ReasonReact.string(
                                    "Included "
                                    ++ string_of_int(call.satoshis)
                                    ++ " satoshis and paid "
                                    ++ string_of_float(
                                         float_of_int(call.paid) /. 1000.0,
                                       )
                                    ++ " at the total cost of "
                                    ++ string_of_int(call.cost)
                                    ++ " msats.",
                                  )}
                               </div>
                             </div>;
                           } else {
                             <div />;
                           }}
                        </div>
                      </div>
                    ),
               ),
             );
           }}
        </div>
        <div className="state">
          <h3> {ReasonReact.string("Current state")} </h3>
          <ReactJSONView
            src={contract.state}
            name="state"
            theme="summerfruit-inverted"
            iconStyle="triangle"
            indentWidth=2
            collapsed=2
            enableClipboard=false
            displayDataTypes=false
            sortKeys=true
          />
        </div>
      </div>
      <div>
        <div className="nextcall">
          <h3> {ReasonReact.string("Make a call")} </h3>
          <div>
            <div>
              <label>
                {ReasonReact.string("Method: ")}
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
                     Array.of_list(
                       contract.code
                       |> API.Helpers.parseMethods
                       |> List.map(m =>
                            <option key=m> {ReasonReact.string(m)} </option>
                          ),
                     ),
                   )}
                </select>
              </label>
            </div>
            <div>
              <label>
                {ReasonReact.string("Satoshis: ")}
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
              </label>
            </div>
            <div className="button">
              <button
                onClick={
                  self.handle((_event, _self) => self.send(PrepareCall))
                }>
                {ReasonReact.string("Prepare call")}
              </button>
            </div>
          </div>
          <div>
            <label>
              {ReasonReact.string("Payload: ")}
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
            </label>
          </div>
        </div>
      </div>
    </div>,
};
