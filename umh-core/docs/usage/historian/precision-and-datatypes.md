# Precision and datatypes

## Numbers lose precision

Numbers and booleans are stored in `value_num`, a `DOUBLE PRECISION` column, which is exact enough for
sensor readings. Integer counters above 2^53 lose their lowest digits, and decimals such as `0.1` are
stored as the nearest binary fraction. Store such values in a text field of the data contract; they
are then kept as they arrived in `value_text`.

## A tag keeps its first datatype

The first value of a tag sets its datatype, number or text. A later value of the other datatype is
dropped and counted on `historian_rows_poisoned`, so each tag's history stays in one column.

`allow_datatype_changes` lifts this rule for a bridge. The tag then stores both datatypes, and you
read it with `coalesce(value_num::text, value_text)`. See
[Runbook: poisoned tags](https://docs.umh.app/benthos-umh/output/historian#runbook-poisoned-tags).
