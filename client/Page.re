open API;

type state = {
  contracts: list(contract),
  view,
}
and view =
  | Loading
  | NotFound
  | Index
  | Contract(contract)
  | NewContract(contract);

type action =
  | UnhandledURL
  | FetchContractsList
  | GotContractsList(list(contract))
  | OpenContract(string)
  | CreateContract;

let component = ReasonReact.reducerComponent("Page");

let make = _children => {
  ...component,
  didMount: self => {
    self.send(FetchContractsList);

    let watcherId =
      ReasonReact.Router.watchUrl(url =>
        switch (url.path) {
        | [] => self.send(FetchContractsList)
        | ["_new"] => self.send(CreateContract)
        | [ctid] => self.send(OpenContract(ctid))
        | _ => self.send(UnhandledURL)
        }
      );
    self.onUnmount(() => ReasonReact.Router.unwatchUrl(watcherId));
  },
  initialState: _state => {contracts: [], view: Index},
  reducer: (action: action, state: state) =>
    switch (action) {
    | UnhandledURL => ReasonReact.Update({...state, view: NotFound})
    | FetchContractsList =>
      ReasonReact.UpdateWithSideEffects(
        {...state, view: Loading},
        (
          self => {
            let _ =
              API.fetchContractsList()
              |> Js.Promise.then_(v =>
                   self.send(GotContractsList(v)) |> Js.Promise.resolve
                 );
            ();
          }
        ),
      )
    | GotContractsList(contracts) =>
      ReasonReact.Update({...state, contracts, view: Index})
    | OpenContract(ctid) =>
      switch (state.contracts |> List.find(ct => ct.id == ctid)) {
      | contract => ReasonReact.Update({...state, view: Contract(contract)})
      | exception Not_found => ReasonReact.Update({...state, view: NotFound})
      }
    | CreateContract =>
      ReasonReact.Update({
        ...state,
        view:
          NewContract({
            id: "",
            code: "",
            name: "",
            readme: "",
            state: Json.Encode.null,
            funds: 0,
            bolt11: None,
          }),
      })
    },
  render: self =>
    <div>
      <div>
        <nav>
          <div className="logo"> <img src="/static/icon.png" /> </div>
          <div>
            <a
              onClick={
                self.handle((_event, _self) => ReasonReact.Router.push("/"))
              }>
              {ReasonReact.string("Etleneum")}
            </a>
          </div>
          <div>
            <a
              onClick={
                self.handle((_event, _self) =>
                  ReasonReact.Router.push("/_new")
                )
              }>
              {ReasonReact.string("Create a smart contract")}
            </a>
          </div>
        </nav>
        <div>
          {
            switch (self.state.view) {
            | Loading =>
              <div id="loading"> {ReasonReact.string("loading...")} </div>
            | NotFound =>
              <div id="error"> {ReasonReact.string("not found")} </div>
            | Index =>
              <div className="contracts">
                {
                  ReasonReact.array(
                    Array.of_list(
                      self.state.contracts
                      |> List.map(c =>
                           <div key={c.id} className="contract-item">
                             <a
                               onClick={
                                 self.handle((_event, _self) =>
                                   ReasonReact.Router.push("/" ++ c.id)
                                 )
                               }>
                               {ReasonReact.string(c.name)}
                             </a>
                           </div>
                         ),
                    ),
                  )
                }
              </div>
            | Contract(c) =>
              <div className="contract">
                <h1> {ReasonReact.string(c.name)} </h1>
                <div>
                  <ReactJSONView
                    src={c.state}
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
            | NewContract(ct) =>
              <div className="new-contract">
                <input value={ct.name} />
                <textarea value={ct.code} />
                <textarea value={ct.readme} />
              </div>
            }
          }
        </div>
      </div>
    </div>,
};
