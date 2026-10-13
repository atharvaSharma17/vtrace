"""Analyze measured V-TRACE trial records. Never treat sampler rows as trials."""
import argparse
import csv
import hashlib
import json
import math
from pathlib import Path
import statistics


def read_jsonl(path):
    if not path.exists():
        return []
    return [json.loads(line) for line in path.read_text().splitlines() if line.strip()]


def describe(values):
    """Independent trial descriptive statistics; no inference for small n."""
    x = sorted(values)
    n = len(x)
    result = dict(n=n, mean=None, median=None, sd=None, cov=None, p95=None, p99=None,
                  inference="insufficient_data")
    if not n:
        return result
    result.update(mean=statistics.mean(x), median=statistics.median(x))
    if n >= 2:
        result["sd"] = statistics.stdev(x)
        result["cov"] = result["sd"] / abs(result["mean"]) if result["mean"] else None
    if n >= 30:
        def quantile(p):
            pos = p * (n - 1)
            a, b = math.floor(pos), math.ceil(pos)
            return x[a] + (x[b] - x[a]) * (pos - a)
        result.update(p95=quantile(.95), p99=quantile(.99), inference="eligible_for_model_checks")
    return result


def bootstrap_mean(x, np, seed=20261013):
    if len(x) < 5:
        return None, None
    rng = np.random.default_rng(seed)
    means = rng.choice(x, (5000, len(x)), replace=True).mean(axis=1)
    return tuple(float(v) for v in np.quantile(means, [.025, .975]))


def analyze(run):
    import numpy as np
    import pandas as pd
    from scipy import stats
    import statsmodels.formula.api as smf
    from statsmodels.stats.anova import anova_lm
    from statsmodels.stats.multitest import multipletests
    import matplotlib
    matplotlib.use("Agg")
    import matplotlib.pyplot as plt

    rows = read_jsonl(run / "trials.jsonl")
    plan = read_jsonl(run / "plan.jsonl")
    host = json.loads((run / "host.json").read_text())
    ids = [r["trial_id"] for r in rows]
    if len(ids) != len(set(ids)):
        raise ValueError("duplicate trial IDs")
    planned = {p["trial_id"]: p for p in plan}
    if len(planned) != len(plan):
        raise ValueError("duplicate plan IDs")
    valid, exclusions = [], []
    for r in rows:
        if r["trial_id"] not in planned:
            raise ValueError("record outside plan")
        p = planned[r["trial_id"]]
        if any(r[k] != p[k] for k in ("condition_id", "backend", "workload", "block_id", "interference_kind")):
            raise ValueError("record factors differ from immutable plan")
        o = r.get("outcome") or {}
        rate = o.get("throughput")
        reason = r.get("invalid_reason", "")
        if (r.get("valid") and r.get("status") == "completed" and
                isinstance(rate, (int, float)) and math.isfinite(rate) and rate > 0 and
                o.get("duration_s", 0) > 0):
            valid.append(dict(trial_id=r["trial_id"], condition_id=r["condition_id"],
                              workload=r["workload"], backend=r["backend"],
                              interference=r["interference_kind"], block=r["block_id"],
                              throughput=rate, unit=o["throughput_unit"], duration=o["duration_s"],
                              environment_confounded=r.get("environment_confounded", True)))
        else:
            exclusions.append(dict(trial_id=r["trial_id"], status=r["status"],
                                   reason=reason or "missing/nonpositive/nonfinite outcome"))
    output = run / "analysis"
    figures = output / "figures"
    figures.mkdir(parents=True, exist_ok=True)
    frame = pd.DataFrame(valid)
    summary, contrasts, models, anomalies, pvalues = [], [], [], [], []
    conditions = {p["condition_id"]: p for p in plan}
    for condition, spec in sorted(conditions.items()):
        group = [r for r in valid if r["condition_id"] == condition]
        values = [r["throughput"] for r in group]
        d = describe(values)
        ci = bootstrap_mean(values, np)
        count = sum(r["condition_id"] == condition for r in rows)
        summary.append(dict(condition_id=condition, workload=spec["workload"],
                            backend=spec["backend"], interference=spec["interference_kind"],
                            unit=group[0]["unit"] if group else None,
                            planned=sum(p["condition_id"] == condition for p in plan),
                            failure_count=count-len(values), attempted=count, **d,
                            mean_ci_low=ci[0], mean_ci_high=ci[1]))
        if len(values) >= 5:
            med = np.median(values)
            mad = np.median(np.abs(np.array(values) - med))
            if mad > 0:
                for row in group:
                    if abs(row["throughput"] - med) > 5 * mad:
                        anomalies.append(dict(trial_id=row["trial_id"], reason="more than 5 MAD from condition median; retained"))

    if not frame.empty:
        for workload, subset in frame.groupby("workload"):
            if subset.unit.nunique() != 1:
                raise ValueError("mixed units within workload")
            baseline_backend = "wsl-process" if "wsl-process" in set(subset.backend) else "linux-process"
            base = subset[(subset.backend == baseline_backend) & (subset.interference == "none")]
            for (backend, interference), group in subset.groupby(["backend", "interference"]):
                pairs = base[["block", "throughput"]].merge(group[["block", "throughput"]], on="block", suffixes=("_base", "_treatment"), validate="one_to_one")
                ratios = (pairs.throughput_base / pairs.throughput_treatment).to_numpy()
                lo, hi = bootstrap_mean(ratios, np)
                contrasts.append(dict(workload=workload, backend=backend, interference=interference,
                                      baseline=f"{baseline_backend}/none", matched_blocks=len(ratios),
                                      normalized_slowdown=float(ratios.mean()) if len(ratios) else None,
                                      ci_low=lo, ci_high=hi,
                                      definition="baseline throughput / treatment throughput; >1 means slowdown"))
            counts = subset.groupby(["backend", "interference"]).size()
            expected_cells = {(p["backend"], p["interference_kind"]) for p in plan if p["workload"] == workload}
            observed_cells = set(counts.index)
            info = dict(workload=workload, n=len(subset), state="insufficient_data")
            if (subset.backend.nunique() >= 2 and subset.interference.nunique() >= 2 and
                    len(counts) == subset.backend.nunique()*subset.interference.nunique() and
                    observed_cells == expected_cells and counts.min() >= 5 and len(subset) >= 30):
                subset = subset.copy()
                subset["log_response"] = np.log(subset.throughput)
                fit = smf.ols("log_response ~ C(backend) * C(interference) + C(block)", data=subset).fit()
                if fit.df_resid >= 8:
                    table = anova_lm(fit, typ=2)
                    residual_ss = float(table.loc["Residual", "sum_sq"])
                    robust = fit.get_robustcov_results(cov_type="HC3")
                    robust_ci = robust.conf_int()
                    info.update(state="fitted", response="log throughput", formula="backend * interference + block",
                                r_squared=float(fit.rsquared), residual_df=float(fit.df_resid),
                                shapiro_p=float(stats.shapiro(fit.resid).pvalue),
                                levene_p=float(stats.levene(*[g.log_response.to_numpy() for _,g in subset.groupby(["backend","interference"])]).pvalue),
                                hc3_coefficients=[dict(term=name, value=float(value), ci_low=float(ci[0]), ci_high=float(ci[1])) for name,value,ci in zip(fit.params.index,robust.params,robust_ci)],
                                terms=[])
                    for term, result in table.iterrows():
                        if term in ("Residual", "C(block)"):
                            continue
                        ss = float(result["sum_sq"])
                        eta = ss/(ss+residual_ss) if ss+residual_ss > 0 else None
                        p = float(result["PR(>F)"])
                        entry = dict(term=term, partial_eta_squared=eta, p=p,
                                     definition="term SS / (term SS + residual SS), type-II ANOVA on log throughput")
                        info["terms"].append(entry)
                        if math.isfinite(p):
                            pvalues.append((entry, p))
            models.append(info)
            labels, distributions = [], []
            for (backend, interference), group in subset.groupby(["backend", "interference"]):
                labels.append(f"{backend}\n{interference}")
                distributions.append(group.duration.to_numpy())
            fig, ax = plt.subplots(figsize=(max(6,len(labels)*2),4))
            ax.boxplot(distributions, tick_labels=labels)
            ax.set_ylabel("Benchmark-reported duration (seconds)")
            ax.set_title(f"{workload}: independent trial timing")
            fig.tight_layout()
            for suffix in ("png", "svg"):
                fig.savefig(figures / f"{workload}_timing.{suffix}")
            plt.close(fig)
            rows_for_workload = [s for s in summary if s["workload"] == workload and s["cov"] is not None]
            if rows_for_workload:
                fig, ax = plt.subplots(figsize=(8,4))
                ax.bar([f'{s["backend"]}\n{s["interference"]}' for s in rows_for_workload], [s["cov"] for s in rows_for_workload])
                ax.set_ylabel("Throughput SD / mean")
                ax.set_title(f"{workload}: trial variability")
                fig.tight_layout()
                for suffix in ("png", "svg"):
                    fig.savefig(figures / f"{workload}_cov.{suffix}")
                plt.close(fig)
            available = [c for c in contrasts if c["workload"] == workload and c["ci_low"] is not None]
            if available:
                fig, ax = plt.subplots(figsize=(8,4))
                values = np.array([c["normalized_slowdown"] for c in available])
                low = values-np.array([c["ci_low"] for c in available])
                high = np.array([c["ci_high"] for c in available])-values
                ax.errorbar(range(len(values)), values, yerr=np.maximum([low,high],0), fmt="o")
                ax.set_xticks(range(len(values)), [f'{c["backend"]}\n{c["interference"]}' for c in available])
                ax.axhline(1,color="gray",linestyle="--")
                ax.set_ylabel("Normalized slowdown, paired block bootstrap 95% CI")
                ax.set_title(f"{workload}: interference/backend contrasts")
                fig.tight_layout()
                for suffix in ("png", "svg"):
                    fig.savefig(figures / f"{workload}_contrasts.{suffix}")
                plt.close(fig)
        if pvalues:
            adjusted = multipletests([p for _,p in pvalues],method="fdr_bh")[1]
            for (entry,_),p in zip(pvalues,adjusted):
                entry["fdr_bh_p"] = float(p)
        effects = [(m["workload"],t) for m in models for t in m.get("terms",[])]
        if effects:
            fig,ax = plt.subplots(figsize=(10,5))
            ax.barh([w+": "+t["term"] for w,t in effects], [t["partial_eta_squared"] or 0 for _,t in effects])
            ax.set_xlabel("Partial eta squared (conditional design association)")
            fig.tight_layout()
            for suffix in ("png","svg"):
                fig.savefig(figures / f"effect_sizes.{suffix}")
            plt.close(fig)
    def write_table(name, records):
        if records:
            with (output / name).open("w", newline="") as f:
                writer = csv.DictWriter(f,fieldnames=list(records[0]))
                writer.writeheader()
                writer.writerows(records)
        else:
            (output/name).write_text("")
    write_table("summary.csv",summary)
    write_table("contrasts.csv",contrasts)
    (output/"models.json").write_text(json.dumps(models,indent=2,allow_nan=False))
    samples = read_jsonl(run / "samples.jsonl")
    quality = dict(planned=len(plan),attempted=len(rows),valid=len(valid),exclusions=exclusions,
                   missing_trials=sorted(set(planned)-set(ids)),anomalies=anomalies,
                   missing_metric_rate=sum(not s.get("available") for s in samples)/len(samples) if samples else None,
                   valid_counts_by_block={str(b):sum(r["block"] == b for r in valid) for b in sorted({p["block_id"] for p in plan})},
                   environment_confounded_count=sum(r.get("environment_confounded",True) for r in rows))
    (output/"quality_report.json").write_text(json.dumps(quality,indent=2,allow_nan=False))
    provenance = hashlib.sha256((run/"trials.jsonl").read_bytes()).hexdigest() if (run/"trials.jsonl").exists() else "no trial file"
    report = f"""# V-TRACE statistical report

Data provenance: measured trial records in `{run.name}/trials.jsonl`; SHA256 `{provenance}`.
Host fingerprint: `{host.get('host_id')}`; kernel `{host.get('kernel')}`.
Scheduled: {len(plan)}; attempted: {len(rows)}; valid: {len(valid)}; excluded: {len(exclusions)}.
Independent trial is the experimental unit. Sampler rows never inflate n.

See summary.csv, contrasts.csv, models.json and quality_report.json for all conditions and exclusions.
Bootstrap uses 5,000 deterministic resamples, with minimum n=5. Contrasts pair by randomized block.
Baseline is wsl-process/none (or linux-process/none); normalized slowdown = baseline throughput / treatment throughput.
P95/P99 describe trial throughput, are suppressed below 30 valid independent trials, and are not per-operation tail latency.
Factor models require a complete factorial design, >=5 valid trials/cell and >=30 total valid trials per workload.
Models use log throughput with block fixed effects, HC3 coefficient CIs, residual normality and variance diagnostics.
Partial eta squared is term SS/(term SS+residual SS); its design association does not identify physical-layer causation.
FDR uses Benjamini-Hochberg across fitted workload factor terms. Inspect interactions before interpreting main effects.

## Limitations

WSL2 is virtualized. Docker Desktop can use a different execution environment. Disk uses caller Linux scratch or an explicitly labeled Docker named volume; network uses backend-namespace loopback.
CPU affinity for local processes and Docker quota are different enforcement treatments. Host Windows activity, thermal state and Docker guest storage quota are incompletely observed.
Anomalies are flagged and retained. Exclusions remain visible. Small n produces insufficient_data, not fabricated inference.
These are fixed-duration workloads: throughput is primary; timing is a duration/QC measure, not fixed-work latency.
This report makes no claim of isolated container overhead or publication-ready causal attribution.
"""
    (output/"statistical_report.md").write_text(report)
    print(f"Analyzed {len(valid)} valid measured trials of {len(plan)} scheduled; outputs: {output}")


if __name__ == "__main__":
    parser = argparse.ArgumentParser()
    parser.add_argument("--run",required=True,type=Path)
    analyze(parser.parse_args().run)
