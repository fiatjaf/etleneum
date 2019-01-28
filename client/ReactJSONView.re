[@bs.module] external reactJsonView: ReasonReact.reactClass = "./JSONView.js";

[@bs.deriving abstract]
type jsProps = {
  src: Js.Json.t,
  name: string,
  theme: string,
  iconStyle: string,
  indentWidth: int,
  collapsed: int,
  enableClipboard: bool,
  displayDataTypes: bool,
  sortKeys: bool,
};

let make =
    (
      ~src,
      ~name,
      ~theme,
      ~iconStyle,
      ~indentWidth,
      ~collapsed,
      ~enableClipboard,
      ~displayDataTypes,
      ~sortKeys,
      children,
    ) =>
  ReasonReact.wrapJsForReason(
    ~reactClass=reactJsonView,
    ~props=
      jsProps(
        ~src,
        ~name,
        ~theme,
        ~iconStyle,
        ~indentWidth,
        ~collapsed,
        ~enableClipboard,
        ~displayDataTypes,
        ~sortKeys,
      ),
    children,
  );
