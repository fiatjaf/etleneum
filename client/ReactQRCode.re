[@bs.module] external reactQR: ReasonReact.reactClass = "./QR.js";

[@bs.deriving abstract]
type jsProps = {
  bgColor: string,
  fgColor: string,
  level: string,
  value: string,
};

let make = (~bgColor, ~fgColor, ~level, ~value, children) =>
  ReasonReact.wrapJsForReason(
    ~reactClass=reactQR,
    ~props=jsProps(~bgColor, ~fgColor, ~level, ~value),
    children,
  );
