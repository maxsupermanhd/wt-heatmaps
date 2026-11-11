// the svg view of the map: pan, zoom, and the pointer for the active tool

export const mapSize = 2048;

const svg = document.getElementById("mapview");
let vb = { x: 0, y: 0, w: mapSize, h: mapSize };
let tool = null;
const pointers = new Map();

function setViewBox() {
	svg.setAttribute("viewBox", `${vb.x} ${vb.y} ${vb.w} ${vb.h}`);
}

// fitViewBox shows the full map, centered. The view box takes the shape of the
// svg element, so one screen pixel covers the same map distance on both axes.
function fitViewBox() {
	const a = svg.clientWidth / svg.clientHeight || 1;
	if (a >= 1) {
		vb = { x: -(mapSize * a - mapSize) / 2, y: 0, w: mapSize * a, h: mapSize };
	} else {
		vb = { x: 0, y: -(mapSize / a - mapSize) / 2, w: mapSize, h: mapSize / a };
	}
	setViewBox();
}

// zoomAt scales the view by factor and keeps the map point under the client
// position x, y in place
function zoomAt(x, y, factor) {
	const rect = svg.getBoundingClientRect();
	const fx = (x - rect.left) / rect.width;
	const fy = (y - rect.top) / rect.height;
	const sx = vb.x + fx * vb.w;
	const sy = vb.y + fy * vb.h;
	vb.w *= factor;
	vb.h *= factor;
	vb.x = sx - fx * vb.w;
	vb.y = sy - fy * vb.h;
	setViewBox();
}

function pan(dx, dy) {
	vb.x -= (dx / svg.clientWidth) * vb.w;
	vb.y -= (dy / svg.clientHeight) * vb.h;
	setViewBox();
}

export function toMap(e) {
	const rect = svg.getBoundingClientRect();
	return {
		x: vb.x + ((e.clientX - rect.left) / rect.width) * vb.w,
		y: vb.y + ((e.clientY - rect.top) / rect.height) * vb.h,
	};
}

// setTool gives one-finger and mouse drags to a tool with down, move, up and
// cancel handlers. null gives them back to pan and zoom.
export function setTool(t) {
	tool = t;
	svg.style.cursor = t != null ? "crosshair" : "";
}

fitViewBox();

// a resize keeps the zoom and the center of the view
new ResizeObserver(() => {
	const a = svg.clientWidth / svg.clientHeight || 1;
	const cy = vb.y + vb.h / 2;
	vb.h = vb.w / a;
	vb.y = cy - vb.h / 2;
	setViewBox();
}).observe(svg);

svg.addEventListener("dblclick", fitViewBox);

svg.addEventListener("wheel", (e) => {
	e.preventDefault();
	zoomAt(e.clientX, e.clientY, e.deltaY < 0 ? 1 / 1.1 : 1.1);
});

svg.addEventListener("pointerdown", (e) => {
	if (tool != null) {
		if (pointers.size == 0) {
			e.preventDefault();
			svg.setPointerCapture(e.pointerId);
			tool.down(e);
		}
		return;
	}
	svg.setPointerCapture(e.pointerId);
	pointers.set(e.pointerId, { x: e.clientX, y: e.clientY });
});

svg.addEventListener("pointermove", (e) => {
	if (tool != null) {
		if (svg.hasPointerCapture(e.pointerId)) {
			tool.move(e);
		}
		return;
	}
	const last = pointers.get(e.pointerId);
	if (last == null) {
		return;
	}
	if (pointers.size == 1) {
		pan(e.clientX - last.x, e.clientY - last.y);
	} else {
		// two fingers: the zoom follows their distance, and the view moves
		// with the point between them
		const other = [...pointers].find(([id]) => id != e.pointerId)[1];
		const before = Math.hypot(last.x - other.x, last.y - other.y);
		const after = Math.hypot(e.clientX - other.x, e.clientY - other.y);
		zoomAt((e.clientX + other.x) / 2, (e.clientY + other.y) / 2, before / after);
		pan((e.clientX - last.x) / 2, (e.clientY - last.y) / 2);
	}
	last.x = e.clientX;
	last.y = e.clientY;
});

svg.addEventListener("pointerup", (e) => {
	if (tool != null && svg.hasPointerCapture(e.pointerId)) {
		tool.up(e);
	}
	pointers.delete(e.pointerId);
});

svg.addEventListener("pointercancel", (e) => {
	if (tool != null) {
		tool.cancel();
	}
	pointers.delete(e.pointerId);
});
