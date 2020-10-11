# etleneum

- a third-party automated escrow
- a platform for hosting complex apps without infrastructure
- a pun with Ethereum
- the Lua Lightning cloud
- the MVPficator of Lightning apps
- a home for your wildest dreams with satoshis

## Build

You need Lua 5.3 and musl to build statically linked binaries.

Install [musl libc](https://musl.libc.org/) then use it to compile [lua5.3](http://www.lua.org/ftp/) with it. What I did was to modify the Lua `Makefile` replacing `gcc` with `musl-gcc` and then compiling normally (with `make posix` instead of `make linux` because of some readline issue). Then symlinking with `ln -s (pwd)/src/liblua.a /usr/lib/musl/lib/liblua5.3.a`.

After all that you can run `make`.

### Less complicated build

You can build without statically linking the C libraries. It requires lua5.3 to be installed and then you run this and it should work:

```
go build -o etleneum
```

## License

Public domain, except you can't use for shitcoins.
