# Configure quality.yaml

`quality.yaml` tells `qk` what your repository owes. It is optional: without it, every check runs advisory and nothing blocks.

## Add a manifest

Create `quality.yaml` at the repository root:

```yaml
version: 1
maturity: bronze
```

`maturity` sets each check's default mode:

- `bronze`: only manifest validity (QK001) blocks; every other check warns.
- `silver`: docs impact, contracts, waivers, generated files and supply chain also block.
- `gold`: every check blocks.

Leave `maturity` out and the repository is bronze.

## Override one check

Under `modes`, set a check to `off`, `advisory` or `enforced`. Use the check key from the [rule reference](../reference/rules.md), not the rule ID:

```yaml
version: 1
maturity: silver
modes:
  docs.impact: advisory
```

## Check the result

```sh
qk check
```

If the manifest is wrong, QK001 fails and names the key, for example `quality.yaml: maturity: value must be one of 'bronze', 'silver', 'gold'`. A key under `modes` that matches no check is a warning, not a failure.

Every key and allowed value: [quality.yaml reference](../reference/manifest.md).
