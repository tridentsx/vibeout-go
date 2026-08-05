// basic.metal is the sole shader for the GPU-API renderer (Metal backend only,
// this dev machine's only target -- see the render-rewrite plan for why MSL
// source is hand-written here instead of going through SDL_shadercross).
//
// The vertex position input is (ndcX, ndcY, cameraSpaceZ): ndcX/ndcY are the
// same screen-space projection already computed on the CPU (matching the old
// painter's-algorithm renderer's math), just mapped to [-1,1] instead of
// pixels. Outputting clip-space (ndcX*z, ndcY*z, depth*z, z) rather than a
// flat-w passthrough means the GPU's own perspective divide both recovers the
// correct screen position AND perspective-correctly interpolates uv/color
// across each triangle -- important once textures are higher-resolution than
// the original PS1 assets, where affine (linear-in-screen-space) interpolation
// would visibly warp.
#include <metal_stdlib>
using namespace metal;

// Matches internal/render/gpu.go's near/far used for the CPU-side clip too.
constant float kNear = 200.0;
constant float kFar = 200000.0;

struct VertexIn {
    float3 position [[attribute(0)]]; // ndcX, ndcY, cameraSpaceZ
    float2 uv       [[attribute(1)]];
    float4 color    [[attribute(2)]];
};

struct VertexOut {
    float4 position [[position]];
    float2 uv;
    float4 color;
};

vertex VertexOut vertexMain(VertexIn in [[stage_in]]) {
    float z = in.position.z;
    float depth = clamp((z - kNear) / (kFar - kNear), 0.0, 1.0);

    VertexOut out;
    out.position = float4(in.position.x * z, in.position.y * z, depth * z, z);
    out.uv = in.uv;
    out.color = in.color;
    return out;
}

fragment float4 fragmentMain(VertexOut in [[stage_in]],
                              texture2d<float> tex [[texture(0)]],
                              sampler samp [[sampler(0)]]) {
    return in.color * tex.sample(samp, in.uv);
}
