#!/usr/bin/env python3
"""Generate benchmark comparison charts for Pheme vs HashiCorp memberlist."""

import matplotlib
matplotlib.use("Agg")
import matplotlib.pyplot as plt
import matplotlib.ticker as ticker
import numpy as np
from pathlib import Path

OUT = Path(__file__).parent.parent / "assets" / "charts"
OUT.mkdir(parents=True, exist_ok=True)

NODES = [5, 10, 50, 100, 500]

PHEME_COLOR = "#2563EB"
ML_COLOR = "#DC2626"
PHEME_LABEL = "Pheme"
ML_LABEL = "memberlist"

FONT_TITLE = dict(fontsize=13, fontweight="bold", color="#111827")
FONT_AXIS = dict(fontsize=11, color="#374151")
FONT_TICK = dict(labelsize=9, colors="#6B7280")
GRID_KW = dict(color="#E5E7EB", linewidth=0.8, linestyle="--")
MARKER_KW = dict(markersize=7, linewidth=2.2)

def style_ax(ax, title, xlabel, ylabel, yscale="linear"):
    ax.set_title(title, **FONT_TITLE, pad=10)
    ax.set_xlabel(xlabel, **FONT_AXIS, labelpad=6)
    ax.set_ylabel(ylabel, **FONT_AXIS, labelpad=6)
    ax.set_yscale(yscale)
    ax.set_xscale("log")
    ax.set_xticks(NODES)
    ax.get_xaxis().set_major_formatter(ticker.ScalarFormatter())
    ax.tick_params(**FONT_TICK)
    ax.grid(axis="both", **GRID_KW)
    ax.spines[["top", "right"]].set_visible(False)
    ax.spines[["left", "bottom"]].set_color("#D1D5DB")
    ax.legend(fontsize=10, framealpha=0.9, edgecolor="#E5E7EB")


# ── 1. Convergence ────────────────────────────────────────────────────────────
pheme_conv  = [0.202, 0.403, 1.82, 3.95, 26.7]
ml_conv     = [0.101, 0.201, 43.2, 58.5, 88.0]

fig, ax = plt.subplots(figsize=(7, 4.5))
ax.plot(NODES, pheme_conv, "o-", color=PHEME_COLOR, label=PHEME_LABEL, **MARKER_KW)
ax.plot(NODES, ml_conv,    "s-", color=ML_COLOR,    label=ML_LABEL,    **MARKER_KW)
ax.annotate("memberlist uses TCP\npush-pull at small sizes",
            xy=(10, 0.201), xytext=(18, 1.5),
            fontsize=8, color="#6B7280",
            arrowprops=dict(arrowstyle="->", color="#9CA3AF", lw=0.9))
style_ax(ax,
    title="Convergence time",
    xlabel="Cluster size (nodes)",
    ylabel="Time (seconds)",
    yscale="log")
ax.yaxis.set_major_formatter(ticker.FuncFormatter(
    lambda y, _: f"{y:g}s"))
fig.tight_layout()
fig.savefig(OUT / "convergence.png", dpi=150, bbox_inches="tight")
plt.close(fig)
print("convergence.png")


# ── 2. Failure detection ──────────────────────────────────────────────────────
pheme_fd = [2.12, 2.22, 2.63, 3.03, 3.73]
ml_fd    = [1.21, 1.41, 1.92, 2.20, 3.00]

fig, ax = plt.subplots(figsize=(7, 4.5))
ax.plot(NODES, pheme_fd, "o-", color=PHEME_COLOR, label=PHEME_LABEL, **MARKER_KW)
ax.plot(NODES, ml_fd,    "s-", color=ML_COLOR,    label=ML_LABEL,    **MARKER_KW)
style_ax(ax,
    title="Failure detection time",
    xlabel="Cluster size (nodes)",
    ylabel="Time (seconds)")
ax.yaxis.set_major_formatter(ticker.FuncFormatter(
    lambda y, _: f"{y:g}s"))
ax.set_ylim(0, 4.5)
fig.tight_layout()
fig.savefig(OUT / "failure_detection.png", dpi=150, bbox_inches="tight")
plt.close(fig)
print("failure_detection.png")


# ── 3. Bandwidth ──────────────────────────────────────────────────────────────
pheme_bw = [1.635, 1.759, 1.883, 1.893, 1.111]
ml_bw    = [0.932, 0.942, 0.993, 1.002, 1.000]

fig, ax = plt.subplots(figsize=(7, 4.5))
ax.plot(NODES, pheme_bw, "o-", color=PHEME_COLOR, label=PHEME_LABEL, **MARKER_KW)
ax.plot(NODES, ml_bw,    "s-", color=ML_COLOR,    label=ML_LABEL,    **MARKER_KW)
ax.annotate("parity at 500 nodes",
            xy=(500, 1.111), xytext=(120, 1.6),
            fontsize=8, color="#6B7280",
            arrowprops=dict(arrowstyle="->", color="#9CA3AF", lw=0.9))
style_ax(ax,
    title="Bandwidth per node (steady state)",
    xlabel="Cluster size (nodes)",
    ylabel="KB/s")
ax.yaxis.set_major_formatter(ticker.FuncFormatter(
    lambda y, _: f"{y:.1f} KB/s"))
ax.set_ylim(0, 2.4)
fig.tight_layout()
fig.savefig(OUT / "bandwidth.png", dpi=150, bbox_inches="tight")
plt.close(fig)
print("bandwidth.png")

print(f"\nAll charts written to {OUT}/")
