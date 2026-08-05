// basic.frag.glsl is the SPIR-V counterpart of basic.metal's fragmentMain. Keep
// the two in step.
//
// The sampler must live in descriptor set 2: SDL_GPU mandates a fixed layout for
// Vulkan shaders -- set 0/1 are the vertex stage's textures/uniforms and set 2/3
// are the fragment stage's. Putting it in set 0 compiles cleanly and then binds
// nothing at runtime.
#version 450

layout(location = 0) in vec2 inUV;
layout(location = 1) in vec4 inColor;

layout(location = 0) out vec4 outColor;

layout(set = 2, binding = 0) uniform sampler2D tex;

void main() {
	vec4 color = inColor * texture(tex, inUV);
	// internal/psx/tim.go's putPixel treats PS1 color-key black (0x0000) as fully
	// transparent rather than a real color, and TIM textures only ever carry that
	// binary 0/255 alpha, so a plain threshold discard reproduces the hardware's
	// punch-through cutout without needing a blend state. Without it, color-keyed
	// pixels paint as opaque black -- TRACK01's billboards and decals, which are
	// mostly color-key around a small logo, became solid black rectangles.
	if (color.a < 0.5) {
		discard;
	}
	outColor = color;
}
