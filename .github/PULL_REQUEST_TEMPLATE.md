Write it like you're explaining the PR to a teammate over coffee — as short as
the PR is big. A tiny fix gets one problem sentence and one or two bullets, a
big feature a few more. Never repeat what the diff already shows. Always say
what you deliberately didn't do.

## Problem

_A sentence or two on why this exists — like explaining it to a teammate._

## What we are shipping

_A few bullets of what changes for the user. A general introduction, not a
code map — the diff already shows the mechanics. Fewer bullets the smaller
the PR._

## What we are NOT shipping

_What you deliberately left out — a deferred refactor, a known limitation.
Only include if there's something to say._

Fixes ENG-####
