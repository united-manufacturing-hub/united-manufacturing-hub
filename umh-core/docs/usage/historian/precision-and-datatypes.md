# Precision and datatypes

This page explains how the Historian stores numbers and text, which values lose precision, and why a
tag keeps the datatype of its first stored value.

## Numbers are stored as DOUBLE PRECISION

The Historian stores numbers and booleans in `value_num`, a `DOUBLE PRECISION` column, and strings
and JSON in `value_text`. `DOUBLE PRECISION` is a binary floating-point type. It stores the nearest
value it can represent, which is close enough for sensor readings.

Two kinds of values do not come back exactly:

- Integer counters above 2^53 (about 9 × 10^15) lose their lowest digits.
- Decimals such as `0.1` are stored as the nearest binary fraction.

Route such tags to a data contract whose field is text. The Historian then stores the value as it
arrived, in `value_text`. See
[Numeric precision](https://docs.umh.app/benthos-umh/output/historian#numeric-precision) in the output
reference.

## A tag keeps its first datatype

The first stored value of a tag sets its datatype, number or text, in `umh.tag.value_type`. A later
value of the other datatype is rejected and dropped, and counted on `historian_rows_poisoned`. The
rest of the batch is written.

This rule keeps each tag's history in one column, so queries and Grafana panels read one column per
tag. Without it, one stray value such as `"N/A"` in a numeric tag would split its history across both
columns.

A data contract from a data model already fixes each field's datatype upstream, so a datatype change
there points to a defect at the source. The generic `_historian` contract has no such check, so a
datatype change there can be genuine, for example a counter that a firmware update turned into a
status string.

`allow_datatype_changes` lifts the rule for a bridge. The tag then stores numbers in `value_num` and
text in `value_text`, and `value_type` keeps the first datatype. Reading the tag then needs
`coalesce(value_num::text, value_text)`. The output reference's
[Runbook: poisoned tags](https://docs.umh.app/benthos-umh/output/historian#runbook-poisoned-tags)
describes when to set it and how to reset a tag's datatype instead.
