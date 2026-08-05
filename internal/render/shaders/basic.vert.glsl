// basic.vert.glsl is the SPIR-V (Vulkan / D3D12-via-shadercross) counterpart of
// basic.metal's vertexMain, for platforms with no Metal backend. Keep the two in
// step: the projection trick below is the whole point of this shader and is
// explained in full in basic.metal.
//
// Input position is (ndcX, ndcY, cameraSpaceZ) -- the screen-space projection is
// already done on the CPU. Emitting clip-space (ndcX*z, ndcY*z, depth*z, z) makes
// the GPU's own perspective divide recover the screen position *and*
// perspective-correctly interpolate uv/color, which affine interpolation would
// visibly warp once textures exceed the original PS1 resolution.
#version 450

layout(location = 0) in vec3 inPosition; // ndcX, ndcY, cameraSpaceZ
layout(location = 1) in vec2 inUV;
layout(location = 2) in vec4 inColor;

layout(location = 0) out vec2 outUV;
layout(location = 1) out vec4 outColor;

// Must match internal/render/gpu.go's near/far, used for the CPU-side clip too.
const float kNear = 200.0;
const float kFar = 200000.0;

void main() {
	float z = inPosition.z;
	float depth = clamp((z - kNear) / (kFar - kNear), 0.0, 1.0);

	gl_Position = vec4(inPosition.x * z, inPosition.y * z, depth * z, z);
	outUV = inUV;
	outColor = inColor;
}
