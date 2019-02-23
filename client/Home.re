let component = ReasonReact.statelessComponent("Home");

[@bs.module "./markdown.js"] external markdown: string => string = "markdown";

let make = _children => {
  ...component,
  render: _self =>
    <div id="home">
      <header>
        <h1> {ReasonReact.string("etleneum")} </h1>
        <p>
          {ReasonReact.string("the centralized smart contract platform")}
        </p>
      </header>
      <section className="items">
        <div>
          <img src="/static/worker-digging-a-hole.svg" />
          <h1> {ReasonReact.string("No proof-of-work")} </h1>
          <p>
            {ReasonReact.string(
               "You think proof-of-work is destroying the enviroment? Etleneum was made for you. Different from Ethereum, which uses proof-of-work, despite all the talk against it, Etleneum doesn't use anything like that. Our consensus process doesn't require that because our network only has one node and data stored in a single Postgres database.",
             )}
          </p>
        </div>
        <div>
          <img src="/static/eye-open.svg" />
          <h1> {ReasonReact.string("Oracle problem solved")} </h1>
          <p>
            {ReasonReact.string(
               "Since our consensus rules and trust model are different from Ethereum and we do not require or use a blockchain at all, we're free to query the time from the OS or make GET requests to get data from the internet. You can use these features in your contracts to create contracts that are not only smart, but also knowledgeable.",
             )}
          </p>
        </div>
        <div>
          <img src="/static/moon-phase-outline.svg" />
          <h1> {ReasonReact.string("Lua, not Sol")} </h1>
          <p>
            {ReasonReact.string(
               "Etleneum's smart contracts are written in Lua, a real programming language, and not Solidity or other bizarre stuff. If you can write one standalone Lua function that modifies a global state you're pretty much ready to launch your Etleneum contract, no need to do a 6-month course on Truffle just to discover everything has changed.",
             )}
          </p>
        </div>
      </section>
      <article
        dangerouslySetInnerHTML={
          "__html":
            markdown(
              "
# How does it work

1. Write a smart contract

  We follow here the same set of bad decisions taken by people who designed Ethereum, thus we define a \"smart contract\" as a collection of methods, funds and state. Methods are [Lua](https://www.lua.org/) functions; state is a JSON/Lua table object that can be accessed and updated by methods; funds are the satoshis you can deposit into the contract whenever you call a method, or extract from the contract by calling `ln.pay`.

2. Call the smart contracts methods
    ",
            ),
        }
      />
      <article
        dangerouslySetInnerHTML={
          "__html":
            markdown(
              "
# FAQ

1. What can do with Etleneum?

Well, you can do a lot of things. We should have a list of examples somewhere, but you can start looking at https://www.stateofthedapps.com/ for inspiration (or maybe not, you'll end up building a game of collectible fake animals).

Oh, remember all our [API](/docs) methods are [CORS](https://developer.mozilla.org/en-US/docs/Web/HTTP/CORS)-enabled, so you can build single-page web applications with just JavaScript and host them for free, but superpower them with Etleneum contracts on the back!

2. This can't be safer than Ethereum. It is centralized!

Yes, you are right. Infura has a lot more of engineers and a much better infrastructure than us, so please don't put a lot of money here. We may get hacked or lose our node with all its channels at anytime and we offer no guarantee whatsoever.
    ",
            ),
        }
      />
    </div>,
};
