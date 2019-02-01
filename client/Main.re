open API;

type state = {
  contracts: list(contract),
  view,
}
and view =
  | Loading
  | NotFound
  | Index
  | ViewContract(contract)
  | ViewNewContract
  | ViewPreparedContract(contract)
  | ViewCall(call, option(result))
  | ViewSimulator
  | ViewSimulatorWithContract(contract);

type action =
  | UnhandledURL
  | FetchContractsList
  | GotContractsList(list(contract))
  | GotContract(contract)
  | FetchPreparedContract(string)
  | GotPreparedContract(contract)
  | LoadContract(string)
  | LoadCall(string)
  | GotCall(call)
  | GotCallResult(result)
  | DispatchCall(string)
  | CreateContract
  | OpenSimulator
  | LoadContractForSimulator(string)
  | OpenSimulatorWithContract(contract);

let component = ReasonReact.reducerComponent("Main");

let make = _children => {
  ...component,
  didMount: self => {
    let initialURL = ReasonReact.Router.dangerouslyGetInitialUrl();
    let handleURL = (url: ReasonReact.Router.url) =>
      switch (url.path) {
      | [] => self.send(FetchContractsList)
      | ["new", ctid] => self.send(FetchPreparedContract(ctid))
      | ["new"] => self.send(CreateContract)
      | ["simulator"] => self.send(OpenSimulator)
      | ["simulator", ctid] => self.send(LoadContractForSimulator(ctid))
      | ["call", callid] => self.send(LoadCall(callid))
      | ["contract", ctid] => self.send(LoadContract(ctid))
      | _ => self.send(UnhandledURL)
      };
    let _ = handleURL(initialURL);
    let watcherId = ReasonReact.Router.watchUrl(handleURL);
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
              API.Contract.list()
              |> Js.Promise.then_(v =>
                   self.send(GotContractsList(v)) |> Js.Promise.resolve
                 );
            ();
          }
        ),
      )
    | GotContractsList(contracts) =>
      ReasonReact.Update({...state, contracts, view: Index})
    | LoadContract(ctid) =>
      ReasonReact.UpdateWithSideEffects(
        {...state, view: Loading},
        (
          self => {
            let _ =
              API.Contract.get(ctid)
              |> Js.Promise.then_(v =>
                   self.send(GotContract(v)) |> Js.Promise.resolve
                 );
            ();
          }
        ),
      )
    | LoadCall(callid) =>
      ReasonReact.UpdateWithSideEffects(
        {...state, view: Loading},
        (
          self => {
            let _ =
              API.Call.get(callid)
              |> Js.Promise.then_(v =>
                   self.send(GotCall(v)) |> Js.Promise.resolve
                 );
            ();
          }
        ),
      )
    | GotContract(contract) =>
      ReasonReact.Update({...state, view: ViewContract(contract)})
    | GotCall(call) =>
      ReasonReact.Update({...state, view: ViewCall(call, None)})
    | GotCallResult(result) =>
      ReasonReact.Update(
        switch (state.view) {
        | ViewCall(call, _) => {
            ...state,
            view: ViewCall(call, Some(result)),
          }
        | _ => state
        },
      )
    | DispatchCall(callid) =>
      ReasonReact.SideEffects(
        (
          self => {
            let _ =
              API.Call.make(callid)
              |> Js.Promise.then_(v =>
                   self.send(GotCallResult(v)) |> Js.Promise.resolve
                 );
            ();
          }
        ),
      )
    | FetchPreparedContract(ctid) =>
      ReasonReact.UpdateWithSideEffects(
        {...state, view: Loading},
        (
          self => {
            let _ =
              API.Contract.get(ctid)
              |> Js.Promise.then_(v =>
                   self.send(GotPreparedContract(v)) |> Js.Promise.resolve
                 );
            ();
          }
        ),
      )
    | GotPreparedContract(contract) =>
      ReasonReact.Update({...state, view: ViewPreparedContract(contract)})
    | CreateContract => ReasonReact.Update({...state, view: ViewNewContract})
    | OpenSimulator => ReasonReact.Update({...state, view: ViewSimulator})
    | LoadContractForSimulator(ctid) =>
      ReasonReact.SideEffects(
        (
          self => {
            let _ =
              API.Contract.get(ctid)
              |> Js.Promise.then_(v =>
                   self.send(OpenSimulatorWithContract(v))
                   |> Js.Promise.resolve
                 );
            ();
          }
        ),
      )
    | OpenSimulatorWithContract(contract) =>
      ReasonReact.Update({
        ...state,
        view: ViewSimulatorWithContract(contract),
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
                  ReasonReact.Router.push("/new")
                )
              }>
              {ReasonReact.string("Create a smart contract")}
            </a>
          </div>
          <div>
            <a
              onClick={
                self.handle((_event, _self) =>
                  ReasonReact.Router.push("/simulator")
                )
              }>
              {ReasonReact.string("Simulator")}
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
              <div>
                <h1>
                  {ReasonReact.string("List of active smart contracts")}
                </h1>
                <div className="contracts">
                  {
                    ReasonReact.array(
                      Array.of_list(
                        self.state.contracts
                        |> List.map((c: contract) =>
                             <div key={c.id} className="contract-item">
                               <a
                                 onClick={
                                   self.handle((_event, _self) =>
                                     ReasonReact.Router.push(
                                       "/contract/" ++ c.id,
                                     )
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
              </div>
            | ViewContract(c) => <ContractPage contract=c />
            | ViewNewContract => <NewContract contract=None />
            | ViewPreparedContract(contract) =>
              <NewContract contract={Some(contract)} />
            | ViewCall(call, result) =>
              <CallPage
                call
                result
                dispatch=(() => self.send(DispatchCall(call.id)))
              />
            | ViewSimulator => <Simulator />
            | ViewSimulatorWithContract(contract) =>
              <Simulator preloadContract=contract />
            }
          }
        </div>
      </div>
    </div>,
};
