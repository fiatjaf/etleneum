let component = ReasonReact.statelessComponent("CallPage");

let make = (~call: API.call, _children) => {
  ...component,
  render: self =>
    <div className="call">
      <h1> {ReasonReact.string("Call " ++ call.id)} </h1>
      {
        switch (call.bolt11) {
        | None =>
          <>
            {ReasonReact.string("This call was already dispatched to ")}
            <a
              className="highlight"
              onClick={
                self.handle((_event, _self) =>
                  ReasonReact.Router.push("/contract/" ++ call.contract_id)
                )
              }>
              {ReasonReact.string(call.contract_id)}
            </a>
            {ReasonReact.string(".")}
          </>
        | Some(bolt11) =>
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
              {ReasonReact.string("Pay the invoice, then dispatch the call.")}
            </div>
            <div className="button">
              <button> {ReasonReact.string("Dispatch!")} </button>
            </div>
          </>
        }
      }
    </div>,
};
