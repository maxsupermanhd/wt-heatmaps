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

const form = document.querySelector("#settingsForm");
var isHeatLoading = false;
form.addEventListener("submit", (e) => {
	if (e.submitter.id != "settingsSubmitBtn") {
		return;
	}
	e.preventDefault();
	let f = new FormData(form);
	if (f.get("level") == "") {
		return;
	}
	if (isHeatLoading) {
		return;
	}
	isHeatLoading = true;
	document.getElementById("settingsSubmitBtnText").innerText = "Loading...";
	// the table on the page counted the filters as they were, so it goes
	setAreaLoaded(false);
	loadHeat(f.get("level"), "/heat?" + new URLSearchParams(f).toString());
	document
		.getElementById("tankmap")
		.setAttribute("href", "/minimap/2048/" + f.get("level"));
});

// The heatmap is one pixel per world meter, so its own pixel size is the grid
// an area selection snaps to. An svg image gives no intrinsic size, so the
// bytes come through fetch and go into the image as a blob. That is one
// request, and the size is known by the time the map is on the screen.
const heatImage = document.getElementById("heat");
const heatLoading = document.getElementById("heatLoading");
// heat is the heatmap on the screen, or null while there is none. A load that
// fails leaves the one before it up, so this always tells what the user sees.
let heat = null;
let heatLoad = 0;

async function loadHeat(level, url) {
	const load = ++heatLoad;
	heatLoading.classList.add("loading");
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
	} finally {
		// the note belongs to the newest load, so an overtaken one leaves it up
		if (load == heatLoad) {
			heatLoading.classList.remove("loading");
		}
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
	document.getElementById("settingsSubmitBtnText").innerText = "Load";
	isHeatLoading = false;
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
			ranked[0][1].btn.click();
			levelSelector.togglePopover();
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
let selectMode = false;
let selectFrom = null;
let areaLoaded = false;

selectBtn.addEventListener("click", () => {
	if (areaLoaded) {
		setAreaLoaded(false);
		return;
	}
	setSelectMode(!selectMode);
});

function setSelectMode(on) {
	selectMode = on;
	svg.style.cursor = on ? "crosshair" : "";
	selectBtn.style.fontWeight = on ? "bold" : "";
}

function setAreaLoaded(on) {
	areaLoaded = on;
	selectBtn.textContent = on ? "Clear area" : "Select area";
	if (!on) {
		selectRect.style.display = "none";
		areaResults.innerHTML = "";
	}
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
	// the box was drawn on the heatmap, so it belongs to the level the heatmap
	// was drawn for, whatever the form says now. The other filters do come from
	// the form, which is what the page promises.
	p.set("level", heat.level);
	p.set("u0", from.x / mapSize);
	p.set("v0", from.y / mapSize);
	p.set("u1", to.x / mapSize);
	p.set("v1", to.y / mapSize);
	// htmx swaps nothing when the server answers an error, and rejects when the
	// request itself fails, so an empty table means the request got nowhere. A
	// swap takes the note with it, and an answer that never came leaves it here.
	areaResults.innerHTML =
		'<div class="loadingNote"><span>area stats loading</span></div>';
	const done = () => {
		areaResults.querySelector(".loadingNote")?.remove();
		setAreaLoaded(areaResults.innerHTML != "");
	};
	htmx.ajax("GET", "/areastats?" + p.toString(), "#areaStatsResults").then(done, done);
});
