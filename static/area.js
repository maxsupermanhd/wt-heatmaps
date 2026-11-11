// area selection: drag a box on the map and get the vehicle stats and the
// kill lines in it, with the filters of the heat on the map

import { mapSize, setTool, toMap } from "./mapview.js";
import { heat } from "./heat.js";
import { isLoading, setLoading } from "./status.js";

const panel = document.getElementById("areaPanel");
const panelToggle = document.getElementById("areaToggle");
const selectBtn = document.getElementById("areaSelectBtn");
const clearBtn = document.getElementById("areaClearBtn");
const selectRect = document.getElementById("areaSelect");
const areaResults = document.getElementById("areaStatsResults");
const areaLines = document.getElementById("mapviewLines");

let selectFrom = null;
let areaLoad = 0;

// snapToGrid puts a map coordinate on the nearest edge between two heatmap
// pixels, and holds it inside the map. cells is the pixel count of the heatmap
// on that axis, one pixel being one world meter.
function snapToGrid(v, cells) {
	const cell = Math.min(Math.max(Math.round((v / mapSize) * cells), 0), cells);
	return (cell / cells) * mapSize;
}

function mapPoint(e) {
	const p = toMap(e);
	// a map with no heatmap on it yet has no grid to snap to
	if (heat != null) {
		p.x = snapToGrid(p.x, heat.w);
		p.y = snapToGrid(p.y, heat.h);
	}
	return p;
}

function drawSelection(to) {
	selectRect.setAttribute("x", Math.min(selectFrom.x, to.x));
	selectRect.setAttribute("y", Math.min(selectFrom.y, to.y));
	selectRect.setAttribute("width", Math.abs(to.x - selectFrom.x));
	selectRect.setAttribute("height", Math.abs(to.y - selectFrom.y));
	selectRect.style.display = "";
}

const selectTool = {
	down(e) {
		selectFrom = mapPoint(e);
		drawSelection(selectFrom);
	},
	move(e) {
		drawSelection(mapPoint(e));
	},
	up(e) {
		const from = selectFrom;
		const to = mapPoint(e);
		setSelectMode(false);
		if (heat == null || from.x == to.x || from.y == to.y) {
			selectRect.style.display = "none";
			return;
		}
		clearBtn.disabled = false;
		loadArea(from, to);
	},
	cancel() {
		selectRect.style.display = "none";
		setSelectMode(false);
	},
};

function setSelectMode(on) {
	selectFrom = null;
	setTool(on ? selectTool : null);
	selectBtn.classList.toggle("active", on);
	selectBtn.setAttribute("aria-pressed", on);
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

export function clearArea() {
	areaLoad++;
	setLoading("area", false);
	setLoading("arrows", false);
	selectRect.style.display = "none";
	areaResults.innerHTML = "";
	areaLines.innerHTML = "";
	clearBtn.disabled = true;
}

clearBtn.addEventListener("click", clearArea);

function loadArea(from, to) {
	const p = new URLSearchParams(heat.params);
	p.set("u0", from.x / mapSize);
	p.set("v0", from.y / mapSize);
	p.set("u1", to.x / mapSize);
	p.set("v1", to.y / mapSize);
	const load = ++areaLoad;

	setLoading("area", true);
	setLoading("arrows", true);
	htmx.ajax("GET", "/data/areastats?" + p.toString(), {
		handler: (_, info) => {
			if (load != areaLoad) {
				return;
			}
			areaResults.innerHTML = info.xhr.response;
			setLoading("area", false);
			if (panelToggle.checkVisibility() && !panel.matches(":popover-open")) {
				panel.showPopover();
			}
		}
	});
	htmx.ajax("GET", "/data/arrows?" + p.toString(), {
		handler: (_, info) => {
			if (load != areaLoad) {
				return;
			}
			// FUCK SVG FUCK SVG FUCK SVG FUCK SVG FUCK SVG FUCK SVG FUCK SVG FUCK SVG
			areaLines.innerHTML = info.xhr.response;
			setLoading("arrows", false);
		}
	});
}
