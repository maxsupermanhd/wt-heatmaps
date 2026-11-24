// the two map images: the minimap, and the heat render over it

import { setLoading } from "./status.ts";

const tankmap = document.getElementById("tankmap") as unknown as SVGImageElement;
const heatImage = document.getElementById("heat") as unknown as SVGImageElement;

export type Heat = { params: URLSearchParams; w: number; h: number; url: string };

// heat is the heat image on the map with the query that made it, or null
export let heat: Heat | null = null;
let heatLoad = 0;

document.getElementById("tankmapBrightnessSlider")!.addEventListener("input", (e) => {
	tankmap.style.filter = `brightness(${(e.target as HTMLInputElement).value}%)`;
});

export function showMinimap(level: string) {
	tankmap.setAttribute("href", "/minimap/2048/" + level + ".png");
}

// loadHeat returns false when the server gives no image
export async function loadHeat(params: URLSearchParams): Promise<boolean> {
	const load = ++heatLoad;
	const probe = new Image();
	setLoading("heat", true);
	try {
		const resp = await fetch("/render/heat?" + params.toString());
		if (resp.status != 200) {
			throw "resp not ok";
		}
		probe.src = URL.createObjectURL(await resp.blob());
		// a 204 answer carries no image, so this rejects
		await probe.decode();
	} catch {
		if (probe.src != "") {
			URL.revokeObjectURL(probe.src);
		}
		if (load != heatLoad) {
			return false;
		}
		// the minimap can show a different map now, so the old heat must go
		if (heat != null) {
			URL.revokeObjectURL(heat.url);
			heat = null;
			heatImage.removeAttribute("href");
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
