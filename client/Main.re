open API;
open Rebase;

type state = {
  contracts: list(contract),
  newcontractstate: NewContract.state,
  view,
}
and view =
  | Loading
  | NotFound
  | ViewHome
  | ViewDocs
  | ContractList
  | ViewContract(contract)
  | ViewNewContract
  | ViewCall(call, option(response))
  | ViewSimulator
  | ViewSimulatorWithContract(contract);

type action =
  | UnhandledURL
  | FetchContractsList
  | GetHome
  | GetDocs
  | GotContractsList(list(contract))
  | GotContract(contract)
  | FetchPreparedContract(string)
  | LoadContract(string)
  | LoadCall(string)
  | GotCall(call)
  | GotCallResult(response)
  | DispatchCall(string)
  | CreateContract
  | OpenSimulator
  | LoadContractForSimulator(string)
  | OpenSimulatorWithContract(contract)
  | NewContractAction(NewContract.action);

let component = ReasonReact.reducerComponent("Main");

let make = _children => {
  ...component,
  didMount: self => {
    let initialURL = ReasonReact.Router.dangerouslyGetInitialUrl();
    let handleURL = (url: ReasonReact.Router.url) =>
      switch (url.path) {
      | [] => self.send(GetHome)
      | ["docs"] => self.send(GetDocs)
      | ["contracts"] => self.send(FetchContractsList)
      | ["new"] => self.send(CreateContract)
      | ["new", ctid] => self.send(FetchPreparedContract(ctid))
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
  initialState: _state => {
    contracts: [],
    newcontractstate: {
      ...NewContract.initialState,
      contract:
        switch (API.LS.getItem("creating-contract")) {
        | None => NewContract.initialState.contract
        | Some(jstr) => jstr |> Js.Json.parseExn |> API.Decode.contract
        },
    },
    view: ContractList,
  },
  reducer: (action: action, state: state) =>
    switch (action) {
    | UnhandledURL => ReasonReact.Update({...state, view: NotFound})
    | GetHome => ReasonReact.Update({...state, view: ViewHome})
    | GetDocs => ReasonReact.Update({...state, view: ViewDocs})
    | FetchContractsList =>
      ReasonReact.UpdateWithSideEffects(
        {...state, view: Loading},
        self => {
          let _ =
            API.Contract.list()
            |> Js.Promise.then_(v =>
                 self.send(GotContractsList(v)) |> Js.Promise.resolve
               );
          ();
        },
      )
    | GotContractsList(contracts) =>
      ReasonReact.Update({...state, contracts, view: ContractList})
    | LoadContract(ctid) =>
      ReasonReact.UpdateWithSideEffects(
        {...state, view: Loading},
        self => {
          let _ =
            API.Contract.get(ctid)
            |> Js.Promise.then_(v =>
                 self.send(GotContract(v)) |> Js.Promise.resolve
               );
          ();
        },
      )
    | LoadCall(callid) =>
      ReasonReact.UpdateWithSideEffects(
        {...state, view: Loading},
        self => {
          let _ =
            API.Call.get(callid)
            |> Js.Promise.then_(v =>
                 self.send(GotCall(v)) |> Js.Promise.resolve
               );
          ();
        },
      )
    | GotContract(contract) =>
      ReasonReact.Update({...state, view: ViewContract(contract)})
    | GotCall(call) =>
      ReasonReact.Update({...state, view: ViewCall(call, None)})
    | GotCallResult(response) =>
      ReasonReact.Update(
        switch (state.view) {
        | ViewCall(call, _) => {
            ...state,
            view: ViewCall(call, Some(response)),
          }
        | _ => state
        },
      )
    | DispatchCall(callid) =>
      ReasonReact.SideEffects(
        self => {
          let _ =
            API.Call.make(callid)
            |> Js.Promise.then_(v =>
                 self.send(GotCallResult(v)) |> Js.Promise.resolve
               );
          ();
        },
      )
    | FetchPreparedContract(ctid) =>
      ReasonReact.UpdateWithSideEffects(
        {...state, view: Loading},
        self => {
          let _ =
            API.Contract.get(ctid)
            |> Js.Promise.then_(v =>
                 self.send(NewContractAction(GotPrepared(v)))
                 |> Js.Promise.resolve
               );
          ();
        },
      )
    | CreateContract => ReasonReact.Update({...state, view: ViewNewContract})
    | OpenSimulator => ReasonReact.Update({...state, view: ViewSimulator})
    | LoadContractForSimulator(ctid) =>
      ReasonReact.SideEffects(
        self => {
          let _ =
            API.Contract.get(ctid)
            |> Js.Promise.then_(v =>
                 self.send(OpenSimulatorWithContract(v))
                 |> Js.Promise.resolve
               );
          ();
        },
      )
    | OpenSimulatorWithContract(contract) =>
      ReasonReact.Update({
        ...state,
        view: ViewSimulatorWithContract(contract),
      })
    | NewContractAction(act) =>
      let newcontractstate = state.newcontractstate;

      switch (act) {
      | Prepare =>
        ReasonReact.UpdateWithSideEffects(
          {...state, view: Loading},
          self => {
            let _ =
              API.Contract.prepare(newcontractstate.contract)
              |> Js.Promise.then_((v: contract) =>
                   self.send(NewContractAction(GotPrepared(v)))
                   |> Js.Promise.resolve
                 );
            ();
          },
        )
      | GotPrepared(contract) =>
        ReasonReact.UpdateWithSideEffects(
          {
            ...state,
            view: ViewNewContract,
            newcontractstate: {
              ...newcontractstate,
              contract,
            },
          },
          _self =>
            if (ReasonReact.Router.dangerouslyGetInitialUrl().path
                == ["new", contract.id]) {
              ();
            } else {
              ReasonReact.Router.push("/new/" ++ contract.id);
            },
        )
      | Initiate =>
        ReasonReact.UpdateWithSideEffects(
          {...state, view: Loading},
          self => {
            let _ =
              API.Contract.make(newcontractstate.contract.id)
              |> Js.Promise.then_((v: response) =>
                   self.send(NewContractAction(GotInitResult(Some(v))))
                   |> Js.Promise.resolve
                 );
            ();
          },
        )
      | GotInitResult(response) =>
        ReasonReact.Update({
          ...state,
          newcontractstate: {
            ...newcontractstate,
            response,
          },
          view: ViewNewContract,
        })
      | _ =>
        let newcontractstate = NewContract.reduceState(act, newcontractstate);

        ReasonReact.UpdateWithSideEffects(
          {...state, newcontractstate},
          _self =>
            API.LS.setItem(
              "creating-contract",
              API.Encode.contract(newcontractstate.contract),
            ),
        );
      };
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
                  ReasonReact.Router.push("/contracts")
                )
              }>
              {ReasonReact.string("List contracts")}
            </a>
          </div>
          <div>
            <a
              onClick={
                self.handle((_event, _self) =>
                  ReasonReact.Router.push("/new")
                )
              }>
              {ReasonReact.string("Create")}
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
          <div>
            <a
              onClick={
                self.handle((_event, _self) =>
                  ReasonReact.Router.push("/docs")
                )
              }>
              {ReasonReact.string("?")}
            </a>
          </div>
        </nav>
        <div>
          {switch (self.state.view) {
           | Loading =>
             <div id="loading"> {ReasonReact.string("loading...")} </div>
           | NotFound =>
             <div id="error"> {ReasonReact.string("not found")} </div>
           | ViewHome => <Home />
           | ViewDocs => <Docs />
           | ContractList =>
             <div>
               <h1>
                 {ReasonReact.string("List of active smart contracts")}
               </h1>
               <div className="contracts">
                 {ReasonReact.array(
                    Array.fromList(
                      self.state.contracts
                      |> List.map((c: contract) =>
                           <div
                             key={c.id}
                             className="contract-item"
                             onClick={
                               self.handle((_event, _self) =>
                                 ReasonReact.Router.push("/contract/" ++ c.id)
                               )
                             }>
                             <h1> {ReasonReact.string(c.name)} </h1>
                             <span>
                               {ReasonReact.string(
                                  String.sub(~from=0, ~length=250, c.readme)
                                  ++ (
                                    if (String.length(c.readme) > 250) {
                                      "…";
                                    } else {
                                      "";
                                    }
                                  ),
                                )}
                             </span>
                             <div>
                               {ReasonReact.string(
                                  string_of_int(c.funds) ++ " msatoshi",
                                )}
                             </div>
                             <div>
                               {ReasonReact.string(
                                  switch (c.ncalls) {
                                  | Some(n) =>
                                    string_of_int(n)
                                    ++ " call"
                                    ++ (
                                      if (n == 1) {
                                        "";
                                      } else {
                                        "s";
                                      }
                                    )
                                  | None => "0 calls"
                                  },
                                )}
                             </div>
                           </div>
                         ),
                    ),
                  )}
               </div>
             </div>
           | ViewContract(c) => <ContractPage contract=c />
           | ViewNewContract =>
             <NewContract
               state={self.state.newcontractstate}
               send={act => self.send(NewContractAction(act))}
             />
           | ViewCall(call, response) =>
             <CallPage
               call
               response
               dispatch={() => self.send(DispatchCall(call.id))}
             />
           | ViewSimulator => <Simulator />
           | ViewSimulatorWithContract(contract) =>
             <Simulator preloadContract=contract />
           }}
        </div>
      </div>
    </div>,
};
