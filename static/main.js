const svg = document.getElementById("mapview");

let vb = {
	x: 0,
	y: 0,
	w: 2048,
	h: 2048,
};
setViewBox();

svg.addEventListener("wheel", (e) => {
	e.preventDefault();
	const factor = 1.1;
	const zoom = e.deltaY < 0 ? 1 / factor : factor;

	const rect = svg.getBoundingClientRect();
	const mx = e.clientX - rect.left;
	const my = e.clientY - rect.top;

	const sx = vb.x + (mx / rect.width) * vb.w;
	const sy = vb.y + (my / rect.height) * vb.h;

	vb.w *= zoom;
	vb.h *= zoom;

	vb.x = sx - (mx / rect.width) * vb.w;
	vb.y = sy - (my / rect.height) * vb.h;

	setViewBox();
});

svg.addEventListener("pointermove", (e) => {
	if (e.buttons == 0) {
		return;
	}
	if (selectFrom != null) {
		drawSelection(svgPoint(e));
		return;
	}
	const dx = (e.movementX / svg.clientWidth) * vb.w;
	const dy = (e.movementY / svg.clientHeight) * vb.h;

	vb.x -= dx;
	vb.y -= dy;

	setViewBox();
});

svg.addEventListener("dblclick", () => {
	vb = { x: 0, y: 0, w: 2048, h: 2048 };
	setViewBox();
});

function setViewBox() {
	svg.setAttribute("viewBox", `${vb.x} ${vb.y} ${vb.w} ${vb.h}`);
}

document.getElementById("tankmapBrightnessSlider").oninput = (e) => {
	for (let v of document.styleSheets) {
		for (let v2 of v.cssRules) {
			if (v2.selectorText == "#tankmap") {
				v2.style.filter = `brightness(${e.target.value}%)`;
			}
		}
	}
};


var loadingIndicatorsKeys = {
	"heat": "Heatmap loading...",
	"arrows": "Area arrows loading...",
	"area": "Area information loading...",
	"testing1": "Testing1...",
	"testing2": "Testing2...",
}
var loadingIndicators = {
	heat: false,
	arrows: false,
	area: false,
	testing1: false,
	testing2: false
};

function updateLoadingIndicators() {
	let ret = ""
	for (const v in loadingIndicators) {
		if (loadingIndicators[v]) {
			ret += loadingIndicatorsKeys[v] + "<br/>"
		}
	}
	document.getElementById("loadingIndicators").innerHTML = ret;
}


const form = document.querySelector("#settingsForm");
form.addEventListener("submit", (e) => {
	if (e.submitter.id != "settingsSubmitBtn") {
		return;
	}
	e.preventDefault();
	let f = new FormData(form);
	if (f.get("level") == "") {
		return;
	}
	if (loadingIndicators.heat) {
		return;
	}
	loadingIndicators.heat = true;
	updateLoadingIndicators();
	document.getElementById("settingsSubmitBtnText").innerText = "Loading...";
	clearAreaStats();

	loadHeat(f.get("level"), "/render/heat?" + new URLSearchParams(f).toString());
	document
		.getElementById("tankmap")
		.setAttribute("href", "/minimap/2048/" + f.get("level"));
});

const heatImage = document.getElementById("heat");
let heat = null;
let heatLoad = 0;

async function loadHeat(level, url) {
	const load = ++heatLoad;
	const probe = new Image();
	try {
		const resp = await fetch(url);
		if (!resp.ok) {
			return;
		}
		probe.src = URL.createObjectURL(await resp.blob());
		// a 204 answer carries no image, so this rejects and the map stays
		await probe.decode();
	} catch {
		if (probe.src != "") {
			URL.revokeObjectURL(probe.src);
		}
		return;
	}
	if (load != heatLoad) {
		URL.revokeObjectURL(probe.src);
		return;
	}
	if (heat != null) {
		URL.revokeObjectURL(heat.url);
	}
	heat = { level, w: probe.naturalWidth, h: probe.naturalHeight, url: probe.src };
	heatImage.setAttribute("href", heat.url);
	loadingIndicators.heat = false;
	updateLoadingIndicators();
	document.getElementById("settingsSubmitBtnText").innerText = "Load";
}

// map selector popover: filter and rank the map rows by what the user types

const levelSelector = document.getElementById("levelSelector");
const levelSearch = document.getElementById("levelSelectorSearch");

if (levelSelector != null && levelSearch != null) {
	const tbody = levelSelector.querySelector("tbody");
	// originalRows keeps the order the server sent, so the list can return to
	// it when the search box empties
	const originalRows = [...tbody.querySelectorAll("tr")];
	const entries = originalRows.map((row) => {
		const button = row.querySelector("button");
		return {
			row,
			name: button.textContent.toLowerCase(),
			id: (button.dataset.levelSelectValue || "").toLowerCase(),
			btn: button,
		};
	});
	levelSearch.addEventListener("input", () => {
		const q = levelSearch.value.trim().toLowerCase();
		if (q == "") {
			for (const e of entries) {
				e.row.hidden = false;
				tbody.appendChild(e.row);
			}
			return;
		}
		const ranked = [];
		for (const e of entries) {
			const score = scoreMapEntry(q, e);
			e.row.hidden = score == null;
			if (score != null) {
				ranked.push([score, e]);
			}
		}
		// sort() is stable, so an equal score keeps the server's color order
		ranked.sort((a, b) => b[0] - a[0]);
		for (const [, e] of ranked) {
			tbody.appendChild(e.row);
		}
	});
	levelSearch.addEventListener("keydown", (e) => {
		if (e.key != 'Enter') {
			return;
		}
		const q = levelSearch.value.trim().toLowerCase();
		if (q == "") {
			return;
		}
		const ranked = [];
		for (const e of entries) {
			const score = scoreMapEntry(q, e);
			if (score != null) {
				ranked.push([score, e]);
			}
		}
		if (ranked.length == 1) {
			document.activeElement.blur();
			ranked[0][1].btn.click();
			document.getElementById("settingsSubmitBtn").click();
		}
	});
}

// scoreMapEntry returns null when neither field matches the query. Otherwise it
// returns the better of the name score and half the id score, so a name match
// wins over an id match of the same quality.
function scoreMapEntry(q, entry) {
	const byName = fuzzyScore(q, entry.name);
	const byId = fuzzyScore(q, entry.id.replace(/[_\-]/g, " "));
	if (byName == null && byId == null) {
		return null;
	}
	return Math.max(byName ?? 0, (byId ?? 0) / 2);
}

// fuzzyScore returns null when text has no reasonable match to query, and a
// number otherwise. A higher number is a better match. The best matches start
// with the query or right after a word break, and leave few extra letters.
// Subsequence matching finds a query spread over a long name. When that comes
// up empty, a near-exact word match covers one typing mistake.
function fuzzyScore(query, text) {
	let qi = 0;
	let start = -1;
	let last = -1;
	let gaps = 0;
	for (let i = 0; i < text.length; i++) {
		if (text[i] != query[qi]) {
			continue;
		}
		if (start < 0) {
			start = i;
		} else if (last >= 0) {
			gaps += i - last - 1;
		}
		last = i;
		qi++;
		if (qi == query.length) {
			break;
		}
	}
	if (qi == query.length) {
		let score = 100;
		if (gaps > 0) {
			score -= gaps * 4;
		}
		score -= text.length - query.length;
		if (start == 0) {
			score += 40;
		} else if (!/[a-z0-9]/.test(text[start - 1])) {
			score += 25;
		}
		if (text == query) {
			score += 120;
		} else if (text.startsWith(query)) {
			score += 60;
		}
		return score;
	}
	return typoWordScore(query, text);
}

// typoWordScore accepts the query when it nearly equals one whole word of the
// text, and turns that equality into a low match score. It exists for swapped
// or mistyped letters ("kurks" for "kursk"), which subsequence matching cannot
// see. The allowed number of edits grows slowly with the query length.
function typoWordScore(query, text) {
	const maxEdits = query.length <= 1 ? 0 : 1 + Math.floor(query.length / 5);
	if (text.length < query.length - maxEdits) {
		return null;
	}
	let best = null;
	for (const word of text.split(/\s+/)) {
		if (word == "") {
			continue;
		}
		const dist = damerauLevenshtein(query, word);
		if (dist > maxEdits) {
			continue;
		}
		const score = 60 - dist * 20;
		if (best == null || score > best) {
			best = score;
		}
	}
	return best;
}

// damerauLevenshtein counts insertions, deletions, substitutions, and adjacent
// transpositions each as one edit. It is the distance two short words differ
// by, and it is small enough here to run on every keystroke.
function damerauLevenshtein(a, b) {
	const n = a.length;
	const m = b.length;
	if (n == 0) {
		return m;
	}
	if (m == 0) {
		return n;
	}
	const d = new Array(n + 1);
	for (let i = 0; i <= n; i++) {
		d[i] = new Array(m + 1);
		d[i][0] = i;
	}
	for (let j = 0; j <= m; j++) {
		d[0][j] = j;
	}
	for (let i = 1; i <= n; i++) {
		for (let j = 1; j <= m; j++) {
			const cost = a[i - 1] == b[j - 1] ? 0 : 1;
			let v = Math.min(
				d[i - 1][j] + 1,
				d[i][j - 1] + 1,
				d[i - 1][j - 1] + cost
			);
			if (i > 1 && j > 1 && a[i - 1] == b[j - 2] && a[i - 2] == b[j - 1]) {
				v = Math.min(v, d[i - 2][j - 2] + 1);
			}
			d[i][j] = v;
		}
	}
	return d[n][m];
}

// area selection, drags a box and asks the server for the vehicles in it

const mapSize = 2048;
const selectBtn = document.getElementById("areaSelectBtn");
const selectRect = document.getElementById("areaSelect");
const areaResults = document.getElementById("areaStatsResults");
const areaLines = document.getElementById("mapviewLines");
let selectMode = false;
let selectFrom = null;

selectBtn.addEventListener("click", () => {
	if (loadingIndicators.area) {
		return;
	}
	setSelectMode(!selectMode);
});

function clearAreaStats() {
	selectRect.style.display = "none";
	areaResults.innerHTML = "";
	areaLines.innerHTML = "";
	selectBtn.textContent = "Select area";
}

function setSelectMode(on) {
	selectMode = on;
	svg.style.cursor = on ? "crosshair" : "";
	selectBtn.style.fontWeight = on ? "bold" : "";
}

// snapToGrid puts a map coordinate on the nearest edge between two heatmap
// pixels, and holds it inside the map. cells is the pixel count of the heatmap
// on that axis, one pixel being one world meter.
function snapToGrid(v, cells) {
	const cell = Math.min(Math.max(Math.round((v / mapSize) * cells), 0), cells);
	return (cell / cells) * mapSize;
}

function svgPoint(e) {
	const rect = svg.getBoundingClientRect();
	const p = {
		x: vb.x + ((e.clientX - rect.left) / rect.width) * vb.w,
		y: vb.y + ((e.clientY - rect.top) / rect.height) * vb.h,
	};
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

svg.addEventListener("pointerdown", (e) => {
	if (!selectMode) {
		return;
	}
	e.preventDefault();
	svg.setPointerCapture(e.pointerId);
	selectFrom = svgPoint(e);
	drawSelection(selectFrom);
});

svg.addEventListener("pointercancel", () => {
	selectFrom = null;
	selectRect.style.display = "none";
	setSelectMode(false);
});

svg.addEventListener("pointerup", (e) => {
	if (selectFrom == null) {
		return;
	}
	svg.releasePointerCapture(e.pointerId);
	const from = selectFrom;
	const to = svgPoint(e);
	selectFrom = null;
	setSelectMode(false);
	if (heat == null || from.x == to.x || from.y == to.y) {
		selectRect.style.display = "none";
		return;
	}
	const p = new URLSearchParams(new FormData(form));
	p.set("level", heat.level);
	p.set("u0", from.x / mapSize);
	p.set("v0", from.y / mapSize);
	p.set("u1", to.x / mapSize);
	p.set("v1", to.y / mapSize);

	loadingIndicators.area = true;
	updateLoadingIndicators();
	htmx.ajax("GET", "/data/areastats?" + p.toString(), "#areaStatsResults").then(
		() => {
			loadingIndicators.area = false;
			updateLoadingIndicators();
		}
	);

	loadingIndicators.arrows = true;
	updateLoadingIndicators();
	htmx.ajax("GET", "/data/arrows?" + p.toString(), {
		handler: (_, info) => {
			// FUCK SVG FUCK SVG FUCK SVG FUCK SVG FUCK SVG FUCK SVG FUCK SVG FUCK SVG
			areaLines.innerHTML = info.xhr.response;
			loadingIndicators.arrows = false;
			updateLoadingIndicators();
		}
	});
});
