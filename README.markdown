Handlebars (Go)
===============

Handlebars provides the power necessary to let you build **semantic templates** effectively with no frustration.
Handlebars is largely compatible with Mustache templates. In most cases it is possible to swap out Mustache with
Handlebars and continue using your current templates.

This repository is a Go port of Handlebars.js 4.7.10: the tokenizer and grammar, whitespace control, the compiler and
runtime, the built-in helpers, partials, inline and block partials, decorators and the compile options, behind a
`handlebars` command that renders a template with partials and JSON data. The language is documented at
[handlebarsjs.com](https://handlebarsjs.com).

Building
--------

Go 1.23 or later, standard library only:

```sh
go build -o handlebars .
go test ./...
```

Usage
-----

```
handlebars [options] TEMPLATE [DATA]
```

`TEMPLATE` is a template file (`-` reads standard input) and `DATA` a JSON file used as the context (an empty object
when it is left out). The rendered text is written to standard output with nothing added.

| option | effect |
| --- | --- |
| `--partial NAME=FILE` | register the contents of FILE as the partial NAME (repeatable) |
| `--data-var NAME=JSON` | add `@NAME` with the given JSON value to the runtime data (repeatable) |
| `--known-helper NAME` / `--unknown-helper NAME` | set `knownHelpers[NAME]` to true / false (repeatable) |
| `--known-helpers-only`, `--strict`, `--assume-objects`, `--prevent-indent`, `--no-escape`, `--explicit-partial-context`, `--compat`, `--ignore-standalone` | the compile option of the same name |
| `--no-data` | compile option `data: false` |
| `--log-level LEVEL` | the logger level (default `info`) |

The command registers these helpers besides the built-in ones (`if`, `unless`, `each`, `with`, `lookup`, `log`):
`upper`, `lower`, `concat`, `join`, `split`, `json`, `safe`, `link`, `length`, `typeof`, `default`, `add`, `mul`,
`div`, `fixed`, `range`, `eq`, `ne`, `lt`, `and`, `or`, `not`, `obj`, `dumphash`, `context`, `optinfo`, `repeat`,
`list`, `bold`, `fail` and `crash`, and the decorators `wrap` and `setdata`. Their values follow JavaScript's
conversions, so output matches Handlebars.js running under Node.js byte for byte.

Exit status: 0 after rendering; 1 when compiling or rendering fails (standard error gets `<error name>: <message>`);
2 for a usage error, an unreadable file or data that is not JSON.

Layout
------

- `internal/ast`: the AST node types.
- `internal/parser`: the lexer, the LALR parser driven by the tables of the original Jison grammar, the AST helpers
  and the whitespace control pass.
- `internal/compiler`: the compiler from the AST to opcodes.
- `internal/runtime`: the interpreter of the opcodes, the runtime (partials, decorators, data frames, property
  access), the logger and the built-in helpers.
- `internal/js`: the JavaScript value model (conversions, number printing, JSON, case mapping, `util.inspect`).
- `internal/cli`: the command line, its helpers and decorators.

Differences Between Handlebars and Mustache
----------------------------------------------
Handlebars adds a couple of additional features to make writing
templates easier and also changes a tiny detail of how partials work.

- [Nested Paths](https://handlebarsjs.com/guide/expressions.html#path-expressions)
- [Helpers](https://handlebarsjs.com/guide/expressions.html#helpers)
- [Block Expressions](https://handlebarsjs.com/guide/block-helpers.html#basic-blocks)
- [Literal Values](https://handlebarsjs.com/guide/expressions.html#literal-segments)
- [Delimited Comments](https://handlebarsjs.com/guide/#template-comments)

Block expressions have the same syntax as mustache sections but should not be confused with one another. Sections are akin to an implicit `each` or `with` statement depending on the input data and helpers are explicit pieces of code that are free to implement whatever behavior they like. The [mustache spec](https://mustache.github.io/mustache.5.html) defines the exact behavior of sections. In the case of name conflicts, helpers are given priority.

### Compatibility

There are a few Mustache behaviors that Handlebars does not implement.
- Handlebars deviates from Mustache slightly in that it does not perform recursive lookup by default. The compile time `compat` flag must be set to enable this functionality. Users should note that there is a performance cost for enabling this flag. The exact cost varies by template, but it's recommended that performance sensitive operations should avoid this mode and instead opt for explicit path references.
- The optional Mustache-style lambdas are not supported. Instead Handlebars provides its own lambda resolution that follows the behaviors of helpers.
- Alternative delimiters are not supported.


Upgrading
---------

See [release-notes.md](release-notes.md) for the history of Handlebars.js releases.

License
-------
Handlebars.js is released under the MIT license; see [LICENSE](LICENSE). The Go port keeps that licence and notice.
