# Location-Based Permissions

## Locations

A location is a position in your company's tree. Level 0, the enterprise, is the only level you have to use. Everything below it is yours to choose, whether you follow ISA-95, KKS, or your own naming.

Location paths use the same dot-separated format as [topic paths](../../usage/unified-namespace/topic-convention.md):

- `ACME` (enterprise)
- `ACME.Munich` (enterprise, site)
- `ACME.Munich.Assembly` (enterprise, site, area)
- `ACME.Munich.Assembly.Line1` (enterprise, site, area, line)
- `ACME.Munich.Assembly.Line1.Cell5` (enterprise, site, area, line, work cell)

Add more levels if your organization needs them.

## Roles

The full capability list is in the [Roles Reference](roles-reference.md).

A user can hold different roles at different locations, for example Admin at `ACME.Munich.Assembly.Line1` and Viewer at `ACME.Munich.Assembly.Line2`.

### Permission inheritance

Permissions inherit downward. Access at `ACME.Munich` also grants access to everything under Munich, including `ACME.Munich.Assembly.Line1.Cell5`.

You can override an inherited permission for a specific location. A user can be a Viewer at `ACME.Munich.Assembly` and an Admin at `ACME.Munich.Assembly.Line1.Cell5` only.