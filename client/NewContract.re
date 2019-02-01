type state = {
  prepared: bool,
  contract: API.contract,
};

type action =
  | EditName(string)
  | EditCode(string)
  | EditReadme(string)
  | Prepare
  | Update
  | GotUpdate(API.contract);

let component = ReasonReact.reducerComponent("NewContract");

let make = (~contract: option(API.contract), _children) => {
  ...component,
  initialState: () =>
    switch (contract) {
    | None => {prepared: false, contract: API.emptyContract}
    | Some(contract) => {prepared: true, contract}
    },
  reducer: (action: action, state: state) => {
    let contract = state.contract;

    switch (action) {
    | EditName(name) =>
      ReasonReact.Update({
        ...state,
        contract: {
          ...contract,
          name,
        },
      })
    | EditCode(code) =>
      ReasonReact.Update({
        ...state,
        contract: {
          ...contract,
          code,
        },
      })
    | EditReadme(readme) =>
      ReasonReact.Update({
        ...state,
        contract: {
          ...contract,
          readme,
        },
      })
    | Prepare =>
      ReasonReact.SideEffects(
        (
          self => {
            let _ =
              API.Contract.prepare(contract)
              |> Js.Promise.then_(v =>
                   self.send(GotUpdate(v)) |> Js.Promise.resolve
                 );
            ();
          }
        ),
      )
    | Update =>
      ReasonReact.SideEffects(
        (
          self => {
            let _ =
              API.Contract.update(contract.id, contract)
              |> Js.Promise.then_(v =>
                   self.send(GotUpdate(v)) |> Js.Promise.resolve
                 );
            ();
          }
        ),
      )
    | GotUpdate(contract) =>
      ReasonReact.UpdateWithSideEffects(
        {prepared: true, contract},
        (_self => ReasonReact.Router.push("/new/" ++ contract.id)),
      )
    };
  },
  render: self =>
    <div className="new-contract">
      <h1>
        {
          if (self.state.prepared) {
            ReasonReact.string("Contract " ++ self.state.contract.id);
          } else {
            ReasonReact.string("Creating a new smart contract");
          }
        }
      </h1>
      <div>
        {ReasonReact.string("Name: ")}
        <div>
          <input
            onChange={
              self.handle((event, _self) =>
                self.send(EditName(event->ReactEvent.Form.target##value))
              )
            }
            value={self.state.contract.name}
          />
        </div>
      </div>
      <div>
        {ReasonReact.string("Lua code: ")}
        <div>
          <textarea
            onChange={
              self.handle((event, _self) =>
                self.send(EditCode(event->ReactEvent.Form.target##value))
              )
            }
            value={self.state.contract.code}
          />
        </div>
      </div>
      <div>
        {ReasonReact.string("README: ")}
        <div>
          <textarea
            onChange={
              self.handle((event, _self) =>
                self.send(EditReadme(event->ReactEvent.Form.target##value))
              )
            }
            value={self.state.contract.readme}
          />
        </div>
      </div>
      <div className="button">
        {
          if (self.state.prepared) {
            <button
              onClick={self.handle((_event, _self) => self.send(Update))}>
              {ReasonReact.string("Update")}
            </button>;
          } else {
            <button
              onClick={self.handle((_event, _self) => self.send(Prepare))}>
              {ReasonReact.string("Prepare")}
            </button>;
          }
        }
      </div>
    </div>,
};
