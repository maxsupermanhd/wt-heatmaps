// the settings form: a load starts only from the Load button or a map pick,
// because a filtered render keeps the server busy for a long time

import { loadHeat, showMinimap } from "./heat.ts";
import { clearArea, loadArea } from "./area.ts";
import { mapSize } from "./mapview.ts";
import "./mappicker.ts";

const form = document.getElementById("settingsForm") as HTMLFormElement;
const panel = document.getElementById("settingsDiv") as HTMLDivElement;
const levelInput = document.getElementById("settingsFormLevelInput") as HTMLInputElement;
const mapSearch = document.getElementById("mapSearch") as HTMLInputElement;
const mapList = document.getElementById("levelSelector") as HTMLDivElement;
const loadBtn = document.getElementById("settingsSubmitBtn") as HTMLButtonElement;
const loadBtnText = document.getElementById("settingsSubmitBtnText") as HTMLSpanElement;

async function load(): Promise<boolean> {
	const f = new FormData(form);
	for (let [name, value] of Array.from(f.entries())) {
		if (value === '') f.delete(name);
	}
	if (!f.get("level")) {
		return false;
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
	// the form has no file inputs, so every value is a string
	const ok = await loadHeat(new URLSearchParams(f as unknown as Record<string, string>));
	loadBtn.disabled = false;
	mapSearch.disabled = false;
	loadBtnText.innerText = ok ? "Load" : "Failed";
	return ok;
}

form.addEventListener("submit", (e) => {
	e.preventDefault();
	if (!loadBtn.disabled) {
		load();
	}
});

form.addEventListener("input", (e) => {
	if ((e.target as HTMLInputElement).name && levelInput.value != "") {
		loadBtn.classList.add("pending");
	}
});

levelInput.addEventListener("change", () => {
	showMinimap(levelInput.value);
	load();
});

// a link from the area Copy link button holds the form values and the area.
// Opening it fills the form, then loads the map and the area as the user would.
const shared = new URLSearchParams(location.search);
const sharedLevel = shared.get("level");
if (sharedLevel != null) {
	// the browser can restore the form values of an earlier visit, and a field
	// that is not in the link must have its default value
	form.reset();
	for (const [name, value] of shared) {
		// u0, v0, u1 and v1 have no form field
		const field = form.elements.namedItem(name) as HTMLInputElement | RadioNodeList | null;
		if (field != null) {
			field.value = value;
		}
	}
	const mapBtn = mapList.querySelector<HTMLButtonElement>(`button[data-level="${CSS.escape(sharedLevel)}"]`);
	mapSearch.value = mapBtn?.dataset.name ?? sharedLevel;
	showMinimap(sharedLevel);
	if ((await load()) && shared.has("u0")) {
		loadArea(
			{ x: Number(shared.get("u0")) * mapSize, y: Number(shared.get("v0")) * mapSize },
			{ x: Number(shared.get("u1")) * mapSize, y: Number(shared.get("v1")) * mapSize },
		);
	}
}
