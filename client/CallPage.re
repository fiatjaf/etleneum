let component = ReasonReact.statelessComponent("CallPage");

let make =
    (~call: API.call, ~result: option(API.result), ~dispatch, _children) => {
  ...component,
  render: self =>
    <div className="call">
      <h1> {ReasonReact.string("Call " ++ call.id)} </h1>
      <p>
        <a
          className="highlight"
          onClick={
            self.handle((_event, _self) =>
              ReasonReact.Router.push("/contract/" ++ call.contract_id)
            )
          }>
          {ReasonReact.string(call.contract_id)}
        </a>
      </p>
      {
        switch (result) {
        | Some({ok, value, error}) =>
          <div>
            {
              ok ?
                <>
                  <div>
                    {ReasonReact.string("Call dispatched successfully.")}
                  </div>
                  <ReactJSONView
                    src=value
                    name="payload"
                    theme="summerfruit-inverted"
                    iconStyle="triangle"
                    indentWidth=2
                    collapsed=5
                    enableClipboard=false
                    displayDataTypes=false
                    sortKeys=true
                  />
                </> :
                <>
                  <div> {ReasonReact.string("Error!")} </div>
                  <div className="error">
                    {ReasonReact.string("\"" ++ error ++ "\"")}
                  </div>
                </>
            }
          </div>
        | None =>
          switch (call.bolt11) {
          | None =>
            <> {ReasonReact.string("This call was already dispatched.")} </>
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
                {
                  ReasonReact.string(
                    "Pay the invoice, then dispatch the call.",
                  )
                }
              </div>
              <div className="button">
                <button onClick={self.handle((_event, _self) => dispatch())}>
                  {ReasonReact.string("Dispatch!")}
                </button>
              </div>
            </>
          }
        }
      }
    </div>,
};
