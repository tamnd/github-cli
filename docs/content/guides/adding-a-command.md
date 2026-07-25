---
title: "Add a command"
description: "Declare an operation once and get a command, a route, an MCP tool, and a URI type."
weight: 60
---

Everything `github` does is one `kit.Handle` registration in `gh/ops.go`, backed by a client method and a record type.
Add those three pieces and every surface updates itself.

## 1. The record

Records live in the `gh` package, one file per family.
Every one embeds `Base`, which carries the kind, the id, the two addresses, the sources it was read from, the tier each field came from, and `Extra`:

```go
type Widget struct {
    Base
    Name  string `json:"name"            table:"name"`
    Owner string `json:"owner,omitempty" table:"owner"`
    Stars *int   `json:"stars,omitempty" table:"stars,num"`
}
```

Two habits matter here.

A count is a pointer, because zero and absent are different answers and a table that shows `0` for "GitHub did not say" is lying.

And every decoder ends with `decodeExtra`, which returns the keys the struct did not claim:

```go
w.setIdentity(KindWidget, id)
w.addSource(resp.URL)
w.addExtra("widgetPayload", decodeExtra(raw, &w))
```

That is the rule the whole tool is built on: decode what you model, keep what you do not.
Without it, a field GitHub adds next month is a field nobody ever sees.
With it, `Extra` names the block it came from and the field shows up the first time anyone looks.

## 2. The client method

```go
func (c *Client) Widget(ctx context.Context, id string) (*Widget, error) {
    var w Widget
    res, err := c.GetJSON(ctx, BaseURL+"/"+id+"/widget", SurfaceRouteJSON, &w)
    if err != nil {
        return nil, err
    }
    w.setIdentity(KindWidget, id)
    w.addSource(res.URL)
    return &w, nil
}
```

`Get`, `GetJSON`, `GetHTML`, `Page`, `Stream`, and `Poll` are the whole client surface, and every one of them handles pacing, retries, the response cache, and the status-to-error mapping, so a method that goes through them inherits the whole policy.
`Poll` is the one for a route that answers 202 while GitHub computes, which is how the contributor graph works.
Which surface you reach for is the real decision, and `github routes` is the record of every one already made.
See [the page plane](/guides/pages/) for what each surface is.

## 3. The operation

An input struct declares the arguments and flags by reflection, and the handler emits records:

```go
type widgetIn struct {
    C   *Client `kit:"inject"`
    Ref string  `kit:"arg" help:"owner/name, or any URL from the repository"`
}

func getWidget(ctx context.Context, in widgetIn, emit func(*Widget) error) error {
    id, err := ResolveRepo(in.Ref)
    if err != nil {
        return err
    }
    w, err := in.C.Widget(ctx, id)
    if err != nil {
        return err
    }
    return emit(w)
}
```

Then register it in the right `register*Ops` function:

```go
kit.Handle(app, kit.OpMeta{
    Name: "widget", Group: "discover", URIType: KindWidget, Single: true, Resolver: true,
    Summary: "Read one widget",
    Args:    []kit.Arg{{Name: "ref", Help: "owner/name, or any URL from the repository"}},
}, getWidget)
```

That is the whole change.
The operation is now:

```bash
github widget golang/go                          # the command
curl 'localhost:7777/v1/widget?ref=golang/go'    # the route, under serve
ant get github://widget/golang/go                # the URI dereference, via a host
```

It is also an MCP tool under `github mcp`, with the summary as its description and the input struct as its schema.

## Input struct tags

| Tag | Meaning |
|---|---|
| `kit:"arg"` | a positional argument |
| `kit:"arg,variadic"` | the rest of the positionals |
| `kit:"flag"` | a flag, named from the field in snake case |
| `kit:"flag,name=min-trust"` | a flag with the name spelled out |
| `kit:"flag,short=r"` | a flag with a short form |
| `kit:"flag,inherit"` | a flag the app already defines globally, like `Limit` |
| `kit:"inject"` | filled in by the app, which is how the client arrives |

Alongside them, `help:"..."`, `default:"..."`, and `enum:"a,b,c"` do what they look like.

## Resolver ops and list ops

Two fields shape how a host treats an operation:

- **`Single: true`** with **`Resolver: true`** marks the canonical one-record fetch for a `URIType`.
  It answers `ant get`.
- **`List: true`** marks a member-lister for a parent resource.
  It answers `ant ls`, and should emit records that are themselves addressable so every member is a URI a host can follow.

## Reference resolution

Handlers do not parse references themselves.
`ResolveRepo` takes anything that names a repository, including a file URL or an issue URL, and gives back `owner/name`.
`ResolveRef(kind, input)` does the same for any other kind.

Both are strict about explicit input and forgiving about guesses.
`github org golang` works because a bare word is a guess that yields to the command.
`github org https://github.com/golang/go` fails, because that URL states its kind and the kind is wrong.

## Errors

Return the kinds from `kit/errs` and every surface reports the same outcome with the same exit code:

```go
return errs.Usage("not a widget id: %q", id)
return errs.NotFound("no widget in %s", repo)
```

Start the message with a plain lowercase word.
The renderer capitalises the first token, so a message that leads with its argument comes back as `"Golang/Go" is a repo` and reads like the tool mangled the input.
`gh/messages_test.go` parses every source file and fails on messages that lead with a quote, a format verb, a URL, or a capital, because the mistake is invisible in review and obvious on screen.

## Add it to the tests

The offline suite has no network in it.
What guards a new command is a live test in `gh/live_test.go`, which runs only under `GITHUB_LIVE=1`:

```go
func TestLiveWidget(t *testing.T) {
    c := liveClient(t)
    ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
    defer cancel()

    w, err := c.Widget(ctx, "gohugoio/hugo")
    if err != nil {
        t.Fatal(err)
    }
    if w.Name == "" {
        t.Error("name missing, the widget payload did not decode")
    }
    if len(w.Extra) > 0 {
        t.Errorf("unmodelled keys: %s", w.Extra)
    }
}
```

Assert shape, not values.
A star count changes hourly and pinning one turns a test into a clock.
The `Extra` check is the one that earns its keep: it is what tells you GitHub added a field.

```bash
make test    # offline, the default
make live    # against github.com
```

If a new command touches the page plane, add a check to `github doctor` too, so a reader whose command came back empty can find out why without reading the source.

## Commands that write bytes

An operation that produces a file or a stream rather than records is not a `kit.Handle`.
Use `kit.Command` and register it in `cli/`, the way `cat`, `readme`, `diff`, `archive`, `rdf`, `export`, and `page` do.
Those are the escape hatch, and there are only a handful of them for a reason: a record is almost always the better answer.
