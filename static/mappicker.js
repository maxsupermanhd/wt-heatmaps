// map picker: the search box ranks the maps as the user types. A pick sets the
// hidden level input and sends a change event from it.

const mapSearch = document.getElementById("mapSearch");
const mapList = document.getElementById("levelSelector");
const levelInput = document.getElementById("settingsFormLevelInput");
const mapEntries = [...mapList.querySelectorAll("button")].map((btn) => ({
	btn,
	name: btn.dataset.name.toLowerCase(),
	id: btn.dataset.level.toLowerCase(),
}));
let mapActive = null;

function openMapList() {
	const r = mapSearch.getBoundingClientRect();
	mapList.style.left = r.left + "px";
	mapList.style.top = r.bottom + 4 + "px";
	mapList.style.width = r.width + "px";
	mapList.style.maxHeight = Math.max(160, window.innerHeight - r.bottom - 16) + "px";
	if (!mapList.matches(":popover-open")) {
		mapList.showPopover();
	}
	mapSearch.setAttribute("aria-expanded", "true");
}

function closeMapList() {
	if (mapList.matches(":popover-open")) {
		mapList.hidePopover();
	}
	mapSearch.setAttribute("aria-expanded", "false");
}

function filterMapList() {
	const q = mapSearch.value.trim().toLowerCase();
	const ranked = [];
	for (const e of mapEntries) {
		const score = q == "" ? 0 : scoreMapEntry(q, e);
		e.btn.hidden = score == null;
		if (score != null) {
			ranked.push([score, e]);
		}
	}
	// sort() is stable, so an equal score keeps the server's order
	ranked.sort((a, b) => b[0] - a[0]);
	for (const [, e] of ranked) {
		mapList.appendChild(e.btn);
	}
	setMapActive(ranked.length > 0 ? ranked[0][1].btn : null);
}

function setMapActive(btn) {
	mapActive?.classList.remove("active");
	mapActive = btn;
	if (btn != null) {
		btn.classList.add("active");
		btn.scrollIntoView({ block: "nearest" });
	}
}

function pickMap(btn) {
	levelInput.value = btn.dataset.level;
	mapSearch.value = btn.dataset.name;
	closeMapList();
	mapSearch.blur();
	levelInput.dispatchEvent(new Event("change", { bubbles: true }));
}

mapSearch.addEventListener("focus", () => {
	mapSearch.select();
	filterMapList();
	openMapList();
});
mapSearch.addEventListener("input", () => {
	filterMapList();
	openMapList();
});
mapSearch.addEventListener("blur", closeMapList);
mapSearch.addEventListener("keydown", (e) => {
	const shown = mapEntries.filter((v) => !v.btn.hidden).map((v) => v.btn);
	const i = shown.indexOf(mapActive);
	switch (e.key) {
		case "ArrowDown":
			e.preventDefault();
			setMapActive(shown[Math.min(i + 1, shown.length - 1)] ?? null);
			return;
		case "ArrowUp":
			e.preventDefault();
			setMapActive(shown[Math.max(i - 1, 0)] ?? null);
			return;
		case "Enter":
			e.preventDefault();
			if (mapActive != null) {
				pickMap(mapActive);
			}
			return;
		case "Escape":
			mapSearch.blur();
	}
});
// mousedown would take the focus from the search box and close the list
// before the click
mapList.addEventListener("mousedown", (e) => e.preventDefault());
mapList.addEventListener("click", (e) => {
	const btn = e.target.closest("button");
	if (btn != null) {
		pickMap(btn);
	}
});
// a phone keyboard resizes the window as it opens
window.addEventListener("resize", () => {
	if (mapList.matches(":popover-open")) {
		openMapList();
	}
});

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
