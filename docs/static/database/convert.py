import collections
import os

import pydbml.classes
from pydbml import PyDBML
from pathlib import Path


def table_to_markdown(dbml_table: pydbml.classes.Table):
    sqltable = []
    sqltable.append(f"### {dbml_table.name}\n")
    sqltable.append(f"{dbml_table.note}\n")
    sqltable.append("| key | data type/format | description |\n")
    sqltable.append("|-----|-----|--------------------------|\n")
    for columns in dbml_table.columns:
        sqltable.append(f"| `{columns.name}` ")
        if columns.not_null or columns.pk:
            sqltable.append(f"| {columns.type} ")
        else:
            sqltable.append(f"| Option<{columns.type}> ")
        if columns.pk:
            sqltable.append(
                f"| [Primary Key](https://www.postgresql.org/docs/current/ddl-constraints.html#DDL-CONSTRAINTS-PRIMARY-KEYS)")
        elif columns.ref_blueprints:
            sqltable.append(f"| ")
            for ref in columns.ref_blueprints:
                sqltable.append(f"References [{ref.table2}.{ref.col2}](#{ref.table2.lower()}) ")
        elif "Name" in columns.name:
            sqltable.append(f"| Name of {dbml_table.name}")
        elif "CreationTime" in columns.name:
            sqltable.append(f"| UNIX creation timestamp of {dbml_table.name}")
        elif "BeginTimestamp" in columns.name:
            sqltable.append(f"| Begin of {dbml_table.name.lower()}")
        elif "EndTimestamp" in columns.name:
            sqltable.append(f"| End of {dbml_table.name.lower()}")
        elif "Timestamp" in columns.name:
            sqltable.append(f"| Timestamp of {dbml_table.name} creation")
        else:
            sqltable.append(f"| {columns.note} ")
        sqltable.append("\n")

    if dbml_table.indexes:
        sqltable.append("#### Indices\n")
        sqltable.append("| keys | type |\n")
        sqltable.append("|-----|-----|\n")
        for index in dbml_table.indexes:
            sqltable.append(f"| ")
            x = len(index.subjects)
            for i, cols in enumerate(index.subjects):
                if i + 1 != x:
                    sqltable.append(f"`{cols.name}`, ")
                else:
                    sqltable.append(f"`{cols.name}` ")
            if index.pk:
                sqltable.append(
                    "| [Primary Key](https://www.postgresql.org/docs/current/ddl-constraints.html#DDL-CONSTRAINTS-PRIMARY-KEYS)")
            elif index.unique:
                sqltable.append(
                    "| [Unique](https://www.postgresql.org/docs/current/ddl-constraints.html#DDL-CONSTRAINTS-UNIQUE-CONSTRAINTS)")
            else:
                sqltable.append("| ")
            sqltable.append("\n")

    sqltable.append("\n\n")
    return dbml_table.name, ''.join(sqltable)


def enum_to_markdown(dbml_enum: pydbml.classes.Enum):
    sqlenum = []
    sqlenum.append(f"### {dbml_enum.name}\n")
    sqlenum.append("| key | description |\n")
    sqlenum.append("|-----|--------------------------|\n")

    for item in dbml_enum.items:
        sqlenum.append(f"| {item.name} ")
        if item.note:
            sqlenum.append(f"| {item.note}")
        sqlenum.append("\n")

    return dbml_enum.name, ''.join(sqlenum)


pre_text = """---
title: "The UMH datamodel / Postgres"
linkTitle: "The UMH datamodel / Postgres"
weight: 2
description: >
    The following model documents our internal postgres tables
---

## TimescaleDB structure

Here is a scheme of the timescaleDB structure:
[{{< imgproc database-model Fit "1792x950" >}}{{< /imgproc >}}](database-model.png)


"""

if __name__ == '__main__':
    parsed = PyDBML(Path('database-model.dbml'))
    tables = []
    for table in parsed.tables:
        tables.append(table_to_markdown(table))

    for enum in parsed.enums:
        tables.append(enum_to_markdown(enum))

    od = collections.OrderedDict(sorted(tables))

    with open("../../content/en/docs/Concepts/datamodel/database/index.md", "w") as file:
        file.write(f"{pre_text}\n")
        for table in od.items():
            file.write(table[1])