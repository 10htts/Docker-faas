import sys
from io import StringIO

import polars as pl


def summarize_csv(text: str) -> str:
    data = text.strip() or "name,value
alpha,10
beta,20
"
    try:
        df = pl.read_csv(StringIO(data))
    except Exception as exc:
        return f"Invalid CSV input: {exc}"

    rows = df.height
    cols = df.width
    summary = [f"rows={rows}", f"cols={cols}"]

    numeric_cols = [c for c, t in zip(df.columns, df.dtypes) if t.is_numeric()]
    if numeric_cols:
        col = numeric_cols[0]
        total = df[col].sum()
        summary.append(f"sum({col})={total}")

    return ", ".join(summary)


def main():
    payload = sys.stdin.read()
    result = summarize_csv(payload)
    print(f"polars summary: {result}")


if __name__ == "__main__":
    main()
