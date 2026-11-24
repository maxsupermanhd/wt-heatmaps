// area selection: drag a box on the map and get the vehicle stats and the
// kill lines in it, with the filters of the heat on the map

import { mapSize, setTool, toMap, type Point, type Tool } from "./mapview.ts";
import { heat } from "./heat.ts";
import { isLoading, setLoading } from "./status.ts";
import htmx from "./vendor/htmx.esm.js";

const panel = document.getElementById("areaPanel") as HTMLDivElement;
const panelToggle = document.getElementById("areaToggle") as HTMLButtonElement;
const selectBtn = document.getElementById("areaSelectBtn") as HTMLButtonElement;
const clearBtn = document.getElementById("areaClearBtn") as HTMLButtonElement;
const copyBtn = document.getElementById("areaCopyBtn") as HTMLButtonElement;
const copyBtnText = copyBtn.textContent;
const selectRect = document.getElementById("areaSelect") as unknown as SVGRectElement;
const areaResults = document.getElementById("areaStatsResults") as HTMLDivElement;
const areaLines = document.getElementById("mapviewLines") as unknown as SVGGElement;

let selectFrom: Point | null = null;
// areaParams is the query of the last area load, and the query of its link
let areaParams: URLSearchParams | null = null;

// snapToGrid puts a map coordinate on the nearest edge between two heatmap
// pixels, and holds it inside the map. cells is the pixel count of the heatmap
// on that axis, one pixel being one world meter.
function snapToGrid(v: number, cells: number): number {
	const cell = Math.min(Math.max(Math.round((v / mapSize) * cells), 0), cells);
	return (cell / cells) * mapSize;
}

function mapPoint(e: PointerEvent): Point {
	const p = toMap(e);
	// a map with no heatmap on it yet has no grid to snap to
	if (heat != null) {
		p.x = snapToGrid(p.x, heat.w);
		p.y = snapToGrid(p.y, heat.h);
	}
	return p;
}

function drawSelection(from: Point, to: Point) {
	selectRect.setAttribute("x", String(Math.min(from.x, to.x)));
	selectRect.setAttribute("y", String(Math.min(from.y, to.y)));
	selectRect.setAttribute("width", String(Math.abs(to.x - from.x)));
	selectRect.setAttribute("height", String(Math.abs(to.y - from.y)));
	selectRect.style.display = "";
}

const selectTool: Tool = {
	down(e) {
		selectFrom = mapPoint(e);
		drawSelection(selectFrom, selectFrom);
	},
	move(e) {
		drawSelection(selectFrom!, mapPoint(e));
	},
	up(e) {
		const from = selectFrom!;
		const to = mapPoint(e);
		setSelectMode(false);
		if (heat == null || from.x == to.x || from.y == to.y) {
			selectRect.style.display = "none";
			return;
		}
		loadArea(from, to);
	},
	cancel() {
		selectRect.style.display = "none";
		setSelectMode(false);
	},
};

function setSelectMode(on: boolean) {
	selectFrom = null;
	setTool(on ? selectTool : null);
	selectBtn.classList.toggle("active", on);
	selectBtn.setAttribute("aria-pressed", String(on));
	// on a small screen the panel covers the map, so it closes for the drag
	if (on && panel.matches(":popover-open")) {
		panel.hidePopover();
	}
}

selectBtn.addEventListener("click", () => {
	if (isLoading("area")) {
		return;
	}
	setSelectMode(!selectBtn.classList.contains("active"));
});

// as in htmx 2, an area query has no time limit
htmx.config.defaultTimeout = 0;

// the responses go in by hand, as htmx would parse the SVG lines as HTML.
// A request that a newer one or Clear area aborted gets no event.
htmx.on(areaResults, "htmx:after:request", (e) => {
	e.preventDefault();
	areaResults.innerHTML = e.detail.ctx.text!;
	setLoading("area", false);
	if (panelToggle.checkVisibility() && !panel.matches(":popover-open")) {
		panel.showPopover();
	}
});
htmx.on(areaResults, "htmx:error", () => {
	areaResults.textContent = "Failed";
	setLoading("area", false);
});
htmx.on(areaLines, "htmx:after:request", (e) => {
	e.preventDefault();
	// FUCK SVG FUCK SVG FUCK SVG FUCK SVG FUCK SVG FUCK SVG FUCK SVG FUCK SVG
	areaLines.innerHTML = e.detail.ctx.text!;
	setLoading("arrows", false);
});
htmx.on(areaLines, "htmx:error", () => setLoading("arrows", false));

export function clearArea() {
	htmx.trigger(areaResults, "htmx:abort");
	htmx.trigger(areaLines, "htmx:abort");
	setLoading("area", false);
	setLoading("arrows", false);
	selectRect.style.display = "none";
	areaResults.innerHTML = "";
	areaLines.innerHTML = "";
	clearBtn.disabled = true;
	copyBtn.disabled = true;
}

clearBtn.addEventListener("click", clearArea);

copyBtn.addEventListener("click", async () => {
	const link = location.origin + location.pathname + "?" + areaParams!.toString();
	try {
		await navigator.clipboard.writeText(link);
	} catch {
		// the clipboard is only available on a secure page, and the user can refuse it
		prompt("Copy this link:", link);
		return;
	}
	copyBtn.textContent = "Copied";
	setTimeout(() => (copyBtn.textContent = copyBtnText), 1500);
});

export function loadArea(from: Point, to: Point) {
	const p = new URLSearchParams(heat!.params);
	p.set("u0", String(from.x / mapSize));
	p.set("v0", String(from.y / mapSize));
	p.set("u1", String(to.x / mapSize));
	p.set("v1", String(to.y / mapSize));
	areaParams = p;
	drawSelection(from, to);
	clearBtn.disabled = false;
	copyBtn.disabled = false;

	setLoading("area", true);
	setLoading("arrows", true);
	// each request has its own source element, which holds its hx-sync
	htmx.ajax("GET", "/data/areastats?" + p.toString(), { source: areaResults });
	htmx.ajax("GET", "/data/arrows?" + p.toString(), { source: areaLines });
}
