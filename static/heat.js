// the two map images: the minimap, and the heat render over it

import { setLoading } from "./status.js";

const tankmap = document.getElementById("tankmap");
const heatImage = document.getElementById("heat");

// heat is the heat image on the map with the query that made it, or null
export let heat = null;
let heatLoad = 0;

document.getElementById("tankmapBrightnessSlider").addEventListener("input", (e) => {
	tankmap.style.filter = `brightness(${e.target.value}%)`;
});

export function showMinimap(level) {
	tankmap.setAttribute("href", "/minimap/2048/" + level + ".png");
}

// loadHeat returns false when the server gives no image
export async function loadHeat(params) {
	const load = ++heatLoad;
	const probe = new Image();
	setLoading("heat", true);
	try {
		const resp = await fetch("/render/heat?" + params.toString());
		if (resp.status != 200) {
			throw "resp not ok";
		}
		probe.src = URL.createObjectURL(await resp.blob());
		// a 204 answer carries no image, so this rejects and the map stays
		await probe.decode();
	} catch {
		if (probe.src != "") {
			URL.revokeObjectURL(probe.src);
		}
		setLoading("heat", false);
		return false;
	}
	if (load != heatLoad) {
		URL.revokeObjectURL(probe.src);
		return true;
	}
	if (heat != null) {
		URL.revokeObjectURL(heat.url);
	}
	heat = { params, w: probe.naturalWidth, h: probe.naturalHeight, url: probe.src };
	heatImage.setAttribute("href", heat.url);
	setLoading("heat", false);
	return true;
}
