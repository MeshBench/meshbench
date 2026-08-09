# UX designs

Rendered from source: `go run ./tools/mockup` regenerates every PNG here. The
designs are generated rather than drawn so a change is a diff, not a redraw.

| Figure | What it shows |
|---|---|
| `01-workbench.png` | The whole shell — map, node inspector, waterfall, run control. Note the persistent "kinder than the air" banner and the provenance footer. |
| `02-path-profile.png` | **The most important panel.** Terrain cut-through with the Fresnel zone and each diffracting edge labelled with its own loss. Answers *why* a link failed. |
| `03-link-budget.png` | Every term as a decibel waterfall, both directions side by side, because reachability is asymmetric. |
| `04-reception-ledger.png` | What every node actually received, including frames the firmware never saw. Five outcomes a packet simulator collapses into one. |
| `05-energy.png` | Battery and solar over 12 months. The output that matters is *which purchase actually fixes it* — usually the panel, not the battery. |
| `06-consoles.png` | Multi-console with synchronised timestamps, and mass commands over the virtual UART (never over the air). |
| `07-interference.png` | External emitters, their effect on the noise floor, and whether a filter would help. |

These correspond to the Feature Catalogue pages in Plane project MSIM.
