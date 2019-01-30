type state = {
  calls: list(API.call),
  callOpen: option(string),
};

type action =
  | LoadCalls
  | GotCalls(list(API.call))
  | OpenCall(string)
  | CloseCall;

let betterdate = datestr =>
  try (String.sub(datestr, 0, 10) ++ " at " ++ String.sub(datestr, 11, 8)) {
  | Invalid_argument(_) => datestr
  };

let component = ReasonReact.reducerComponent("ContractPage");

let make = (~contract: API.contract, _children) => {
  ...component,
  didMount: self => self.send(LoadCalls),
  initialState: () => {calls: [], callOpen: None},
  reducer: (action: action, state: state) =>
    switch (action) {
    | LoadCalls =>
      ReasonReact.SideEffects(
        (
          self => {
            let _ =
              API.fetchCalls(contract.id)
              |> Js.Promise.then_(v =>
                   self.send(GotCalls(v)) |> Js.Promise.resolve
                 );
            ();
          }
        ),
      )
    | GotCalls(calls) => ReasonReact.Update({...state, calls})
    | OpenCall(callid) =>
      ReasonReact.Update({...state, callOpen: Some(callid)})
    | CloseCall => ReasonReact.Update({...state, callOpen: None})
    },
  render: self =>
    <div className="contract">
      <header>
        <h1> {ReasonReact.string(contract.name)} </h1>
        <aside>
          {ReasonReact.string("since ")}
          <time dateTime={contract.created_at}>
            {ReasonReact.string(contract.created_at |> betterdate)}
          </time>
        </aside>
      </header>
      <div>
        <div className="readme">
          <h3> {ReasonReact.string("Readme")} </h3>
          {ReasonReact.string(contract.readme)}
        </div>
        <div>
          <h3> {ReasonReact.string("Code")} </h3>
          <textarea readOnly=true value={contract.code} />
        </div>
      </div>
      <div />
      <div>
        <div className="calls">
          <h3> {ReasonReact.string("Last calls")} </h3>
          {
            if (self.state.calls |> List.length == 0) {
              <p>
                {
                  ReasonReact.string(
                    "No calls were made to this contract yet.",
                  )
                }
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
                             if (self.state.callOpen == Some(call.id)) {
                               " open";
                             } else {
                               "";
                             }
                           )
                         }>
                         <div>
                           <a
                             onClick={
                               self.handle((_event, self) =>
                                 if (self.state.callOpen == Some(call.id)) {
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
                           {
                             if (self.state.callOpen == Some(call.id)) {
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
                                   {
                                     ReasonReact.string(
                                       "Included "
                                       ++ string_of_int(call.satoshis)
                                       ++ " satoshis and withdrew "
                                       ++ string_of_int(call.paid)
                                       ++ " at the total cost of "
                                       ++ string_of_int(call.cost)
                                       ++ " msats.",
                                     )
                                   }
                                 </div>
                               </div>;
                             } else {
                               <div />;
                             }
                           }
                         </div>
                       </div>
                     ),
                ),
              );
            }
          }
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
    </div>,
};
