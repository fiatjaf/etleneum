type state = {
  contract: API.contract,
  result: option(API.result),
};

type action =
  | EditName(string)
  | EditCode(string)
  | EditReadme(string)
  | Prepare
  | GotPrepared(API.contract)
  | Initiate
  | GotInitResult(option(API.result))
  | GoBack;

let initialState = {contract: API.emptyContract, result: None};

let reduceState = (action: action, state: state) => {
  let contract = state.contract;

  switch (action) {
  | EditName(name) => {
      ...state,
      contract: {
        ...contract,
        name,
      },
    }
  | EditCode(code) => {
      ...state,
      contract: {
        ...contract,
        code,
      },
    }
  | EditReadme(readme) => {
      ...state,
      contract: {
        ...contract,
        readme,
      },
    }
  | GoBack => {...state, result: None}
  | _ => state
  };
};

let component = ReasonReact.statelessComponent("NewContract");

let make = (~send, ~state, _children) => {
  ...component,
  render: self =>
    <div className="new-contract">
      <h1>
        {
          if (state.result != None) {
            ReasonReact.string("Contract " ++ state.contract.id);
          } else {
            ReasonReact.string("Creating a new smart contract");
          }
        }
      </h1>
      {
        switch (state.result) {
        | Some({ok, error}) =>
          <div>
            {
              ok ?
                <div>
                  {ReasonReact.string("Contract created successfully. ")}
                  <a
                    className="highlight"
                    onClick={
                      self.handle((_event, _self) =>
                        ReasonReact.Router.push(
                          "/contract/" ++ state.contract.id,
                        )
                      )
                    }>
                    {ReasonReact.string(state.contract.id)}
                  </a>
                </div> :
                <>
                  <div> {ReasonReact.string("Error!")} </div>
                  <div className="error">
                    {ReasonReact.string("\"" ++ error ++ "\"")}
                  </div>
                  <div>
                    <a
                      className="highlight"
                      onClick={self.handle((_event, _self) => send(GoBack))}>
                      {ReasonReact.string("back")}
                    </a>
                  </div>
                </>
            }
          </div>
        | None =>
          switch (state.contract.invoice_paid, state.contract.bolt11) {
          | (false, None) =>
            <>
              <label>
                <div> {ReasonReact.string("Name: ")} </div>
                <div>
                  <input
                    onChange={
                      self.handle((event, _self) =>
                        send(EditName(event->ReactEvent.Form.target##value))
                      )
                    }
                    value={state.contract.name}
                  />
                </div>
              </label>
              <div className="row">
                <label>
                  <div> {ReasonReact.string("Lua code: ")} </div>
                  <div>
                    <textarea
                      onChange={
                        self.handle((event, _self) =>
                          send(
                            EditCode(event->ReactEvent.Form.target##value),
                          )
                        )
                      }
                      value={state.contract.code}
                    />
                  </div>
                </label>
                <label>
                  <div> {ReasonReact.string("README: ")} </div>
                  <div>
                    <textarea
                      onChange={
                        self.handle((event, _self) =>
                          send(
                            EditReadme(event->ReactEvent.Form.target##value),
                          )
                        )
                      }
                      value={state.contract.readme}
                    />
                  </div>
                </label>
              </div>
              <div className="button">
                <button
                  onClick={self.handle((_event, _self) => send(Prepare))}>
                  {ReasonReact.string("Prepare")}
                </button>
              </div>
            </>
          | (false, Some(bolt11)) =>
            <>
              <div className="bolt11">
                <ReactQRCode
                  fgColor="#333333"
                  bgColor="#efefef"
                  level="L"
                  value=bolt11
                />
                <p> {ReasonReact.string(bolt11)} </p>
              </div>
              <div>
                {
                  ReasonReact.string(
                    "Pay the invoice above, then click the button to initiate the contract.",
                  )
                }
              </div>
              <div className="button">
                <button
                  onClick={self.handle((_event, _self) => send(Initiate))}>
                  {ReasonReact.string("Initiate!")}
                </button>
              </div>
            </>
          | (true, _) =>
            <>
              <div className="bolt11">
                <p>
                  {
                    ReasonReact.string(
                      "Invoice paid already. Click the button to initiate the contract.",
                    )
                  }
                </p>
              </div>
              <div className="button">
                <button
                  onClick={self.handle((_event, _self) => send(Initiate))}>
                  {ReasonReact.string("Initiate!")}
                </button>
              </div>
            </>
          }
        }
      }
    </div>,
};
