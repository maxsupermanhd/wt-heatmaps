// the settings form: a load starts only from the Load button or a map pick,
// because a filtered render keeps the server busy for a long time

import { loadHeat, showMinimap } from "./heat.js";
import { clearArea } from "./area.js";
import "./mappicker.js";

const form = document.getElementById("settingsForm");
const panel = document.getElementById("settingsDiv");
const levelInput = document.getElementById("settingsFormLevelInput");
const mapSearch = document.getElementById("mapSearch");
const loadBtn = document.getElementById("settingsSubmitBtn");
const loadBtnText = document.getElementById("settingsSubmitBtnText");

async function load() {
	const f = new FormData(form);
	for (let [name, value] of Array.from(f.entries())) {
		if (value === '') f.delete(name);
	}
	if (!f.get("level")) {
		return;
	}
	clearArea();
	loadBtn.disabled = true;
	mapSearch.disabled = true;
	loadBtn.classList.remove("pending");
	loadBtnText.innerText = "Loading...";
	// on a small screen the panel covers the map
	if (panel.matches(":popover-open")) {
		panel.hidePopover();
	}
	const ok = await loadHeat(new URLSearchParams(f));
	loadBtn.disabled = false;
	mapSearch.disabled = false;
	loadBtnText.innerText = ok ? "Load" : "Failed";
}

form.addEventListener("submit", (e) => {
	e.preventDefault();
	if (!loadBtn.disabled) {
		load();
	}
});

form.addEventListener("input", (e) => {
	if (e.target.name && levelInput.value != "") {
		loadBtn.classList.add("pending");
	}
});

levelInput.addEventListener("change", () => {
	showMinimap(levelInput.value);
	load();
});
