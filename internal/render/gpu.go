package render

import (
	"bytes"
	_ "embed"
	"fmt"
	"image"
	"image/png"
	"unsafe"

	"github.com/Zyko0/go-sdl3/sdl"
	"github.com/tridentsx/wipeout-go/internal/assets"
)

//go:embed shaders/basic.metal
var basicShaderSource []byte

// depthNear/depthFar bound the depth-buffer normalization done in
// shaders/basic.metal (kept in sync there -- see that file's comment). near
// matches the existing CPU-side near-plane clip in track.go/model.go; far is
// comfortably beyond any TRACK01 sightline once TRACK.VEW visibility culling
// (track.go's visibleFaces) has already dropped distant/behind-camera faces.
const (
	depthNear = float32(200)
	depthFar  = float32(200000)
)

// maxFrameVertices bounds the shared per-frame vertex buffer. Current scenes
// are a couple hundred triangles (ship: 160 polygons: a few visible track
// sections); this leaves generous headroom without needing to grow buffers.
const maxFrameVertices = 1 << 16

// Vertex is one GPU-submitted vertex. X/Y are NDC (screen-space projection
// already computed on the CPU, mapped to [-1,1] instead of pixels); Z is the
// raw camera-space depth -- the vertex shader derives both the perspective
// divide and the normalized depth-buffer value from it, rather than this
// struct carrying a separately precomputed depth (see shaders/basic.metal).
type Vertex struct {
	X, Y, Z    float32
	U, V       float32
	R, G, B, A float32
}

// Device owns the GPU device, the single depth-tested pipeline shared by
// track and ship rendering, and the buffers/textures that persist across
// frames.
type Device struct {
	device   *sdl.GPUDevice
	window   *sdl.Window
	pipeline *sdl.GPUGraphicsPipeline

	width, height int32
	colorTexture  *sdl.GPUTexture // offscreen target; blitted to the swapchain each frame
	depthTexture  *sdl.GPUTexture

	vertexBuffer   *sdl.GPUBuffer
	transferBuffer *sdl.GPUTransferBuffer

	whiteTexture *sdl.GPUTexture // fallback for untextured (flat-color) triangles
	sampler      *sdl.GPUSampler
}

// Frame accumulates one frame's triangles, grouped by texture, between
// BeginFrame and Present. A nil texture group draws with the device's white
// fallback texture, matching the old renderer's untextured-color path.
type Frame struct {
	cmdbuf    *sdl.GPUCommandBuffer
	swapchain *sdl.SwapchainTexture
	byTexture map[*sdl.GPUTexture][]Vertex
}

// NewDevice creates the GPU device, claims window for it, and builds the
// fixed-size (window stays non-resizable) depth-tested pipeline and its
// supporting textures/buffers.
func NewDevice(window *sdl.Window, width, height int32) (*Device, error) {
	gd, err := sdl.CreateGPUDevice(sdl.GPU_SHADERFORMAT_MSL, false, "")
	if err != nil {
		return nil, fmt.Errorf("render: create GPU device: %w", err)
	}
	if err := gd.ClaimWindow(window); err != nil {
		gd.Destroy()
		return nil, fmt.Errorf("render: claim window for GPU: %w", err)
	}

	d := &Device{device: gd, window: window, width: width, height: height}
	if err := d.init(); err != nil {
		d.Destroy()
		return nil, err
	}
	return d, nil
}

func (d *Device) init() error {
	vertexShader, err := newShader(d.device, basicShaderSource, "vertexMain", sdl.GPU_SHADERSTAGE_VERTEX, 0)
	if err != nil {
		return err
	}
	defer d.device.ReleaseShader(vertexShader)
	fragmentShader, err := newShader(d.device, basicShaderSource, "fragmentMain", sdl.GPU_SHADERSTAGE_FRAGMENT, 1)
	if err != nil {
		return err
	}
	defer d.device.ReleaseShader(fragmentShader)

	d.colorTexture, err = d.device.CreateTexture(&sdl.GPUTextureCreateInfo{
		Type:              sdl.GPU_TEXTURETYPE_2D,
		Width:             uint32(d.width),
		Height:            uint32(d.height),
		LayerCountOrDepth: 1,
		NumLevels:         1,
		SampleCount:       sdl.GPU_SAMPLECOUNT_1,
		Format:            sdl.GPU_TEXTUREFORMAT_R8G8B8A8_UNORM,
		Usage:             sdl.GPU_TEXTUREUSAGE_SAMPLER | sdl.GPU_TEXTUREUSAGE_COLOR_TARGET,
	})
	if err != nil {
		return fmt.Errorf("render: create color texture: %w", err)
	}
	d.depthTexture, err = d.device.CreateTexture(&sdl.GPUTextureCreateInfo{
		Type:              sdl.GPU_TEXTURETYPE_2D,
		Width:             uint32(d.width),
		Height:            uint32(d.height),
		LayerCountOrDepth: 1,
		NumLevels:         1,
		SampleCount:       sdl.GPU_SAMPLECOUNT_1,
		Format:            sdl.GPU_TEXTUREFORMAT_D16_UNORM,
		Usage:             sdl.GPU_TEXTUREUSAGE_DEPTH_STENCIL_TARGET,
	})
	if err != nil {
		return fmt.Errorf("render: create depth texture: %w", err)
	}

	d.pipeline, err = d.device.CreateGraphicsPipeline(&sdl.GPUGraphicsPipelineCreateInfo{
		TargetInfo: sdl.GPUGraphicsPipelineTargetInfo{
			ColorTargetDescriptions: []sdl.GPUColorTargetDescription{
				{Format: sdl.GPU_TEXTUREFORMAT_R8G8B8A8_UNORM},
			},
			HasDepthStencilTarget: true,
			DepthStencilFormat:    sdl.GPU_TEXTUREFORMAT_D16_UNORM,
		},
		DepthStencilState: sdl.GPUDepthStencilState{
			EnableDepthTest:  true,
			EnableDepthWrite: true,
			CompareOp:        sdl.GPU_COMPAREOP_LESS,
			WriteMask:        0xFF,
		},
		RasterizerState: sdl.GPURasterizerState{
			// track.go/model.go already backface-cull on the CPU (backFacing);
			// the GPU pipeline doesn't need to duplicate that.
			CullMode:  sdl.GPU_CULLMODE_NONE,
			FillMode:  sdl.GPU_FILLMODE_FILL,
			FrontFace: sdl.GPU_FRONTFACE_COUNTER_CLOCKWISE,
		},
		VertexInputState: sdl.GPUVertexInputState{
			VertexBufferDescriptions: []sdl.GPUVertexBufferDescription{
				{Slot: 0, InputRate: sdl.GPU_VERTEXINPUTRATE_VERTEX, Pitch: uint32(unsafe.Sizeof(Vertex{}))},
			},
			VertexAttributes: []sdl.GPUVertexAttribute{
				{BufferSlot: 0, Location: 0, Format: sdl.GPU_VERTEXELEMENTFORMAT_FLOAT3, Offset: 0},
				{BufferSlot: 0, Location: 1, Format: sdl.GPU_VERTEXELEMENTFORMAT_FLOAT2, Offset: uint32(unsafe.Sizeof(float32(0)) * 3)},
				{BufferSlot: 0, Location: 2, Format: sdl.GPU_VERTEXELEMENTFORMAT_FLOAT4, Offset: uint32(unsafe.Sizeof(float32(0)) * 5)},
			},
		},
		PrimitiveType:  sdl.GPU_PRIMITIVETYPE_TRIANGLELIST,
		VertexShader:   vertexShader,
		FragmentShader: fragmentShader,
	})
	if err != nil {
		return fmt.Errorf("render: create graphics pipeline: %w", err)
	}

	d.vertexBuffer, err = d.device.CreateBuffer(&sdl.GPUBufferCreateInfo{
		Usage: sdl.GPU_BUFFERUSAGE_VERTEX,
		Size:  maxFrameVertices * uint32(unsafe.Sizeof(Vertex{})),
	})
	if err != nil {
		return fmt.Errorf("render: create vertex buffer: %w", err)
	}
	d.transferBuffer, err = d.device.CreateTransferBuffer(&sdl.GPUTransferBufferCreateInfo{
		Usage: sdl.GPU_TRANSFERBUFFERUSAGE_UPLOAD,
		Size:  maxFrameVertices * uint32(unsafe.Sizeof(Vertex{})),
	})
	if err != nil {
		return fmt.Errorf("render: create vertex transfer buffer: %w", err)
	}

	d.sampler, err = d.device.CreateSampler(&sdl.GPUSamplerCreateInfo{
		MinFilter:    sdl.GPU_FILTER_LINEAR,
		MagFilter:    sdl.GPU_FILTER_LINEAR,
		MipmapMode:   sdl.GPU_SAMPLERMIPMAPMODE_LINEAR,
		AddressModeU: sdl.GPU_SAMPLERADDRESSMODE_CLAMP_TO_EDGE,
		AddressModeV: sdl.GPU_SAMPLERADDRESSMODE_CLAMP_TO_EDGE,
		AddressModeW: sdl.GPU_SAMPLERADDRESSMODE_CLAMP_TO_EDGE,
	})
	if err != nil {
		return fmt.Errorf("render: create sampler: %w", err)
	}

	white, err := d.NewTexture(1, 1, []byte{255, 255, 255, 255})
	if err != nil {
		return err
	}
	d.whiteTexture = white
	return nil
}

func newShader(device *sdl.GPUDevice, source []byte, entrypoint string, stage sdl.GPUShaderStage, samplers uint32) (*sdl.GPUShader, error) {
	shader, err := device.CreateGPUShader(&sdl.GPUShaderCreateInfo{
		Code:        source,
		Entrypoint:  entrypoint,
		Format:      sdl.GPU_SHADERFORMAT_MSL,
		Stage:       stage,
		NumSamplers: samplers,
	})
	if err != nil {
		return nil, fmt.Errorf("render: compile %s shader: %w", entrypoint, err)
	}
	return shader, nil
}

// NewTexture uploads width*height RGBA8 pixels (4 bytes/pixel, row-major,
// straight alpha) to a new sampled GPU texture. Used both for the device's
// white fallback and by track.go for decoded track tile textures.
func (d *Device) NewTexture(width, height int, pixels []byte) (*sdl.GPUTexture, error) {
	tex, err := d.device.CreateTexture(&sdl.GPUTextureCreateInfo{
		Type:              sdl.GPU_TEXTURETYPE_2D,
		Width:             uint32(width),
		Height:            uint32(height),
		LayerCountOrDepth: 1,
		NumLevels:         1,
		SampleCount:       sdl.GPU_SAMPLECOUNT_1,
		Format:            sdl.GPU_TEXTUREFORMAT_R8G8B8A8_UNORM,
		Usage:             sdl.GPU_TEXTUREUSAGE_SAMPLER,
	})
	if err != nil {
		return nil, fmt.Errorf("render: create texture: %w", err)
	}

	size := uint32(width * height * 4)
	transfer, err := d.device.CreateTransferBuffer(&sdl.GPUTransferBufferCreateInfo{
		Usage: sdl.GPU_TRANSFERBUFFERUSAGE_UPLOAD, Size: size,
	})
	if err != nil {
		d.device.ReleaseTexture(tex)
		return nil, fmt.Errorf("render: create texture transfer buffer: %w", err)
	}
	defer d.device.ReleaseTransferBuffer(transfer)

	ptr, err := d.device.MapTransferBuffer(transfer, false)
	if err != nil {
		d.device.ReleaseTexture(tex)
		return nil, fmt.Errorf("render: map texture transfer buffer: %w", err)
	}
	copy(unsafe.Slice((*byte)(unsafe.Pointer(ptr)), size), pixels)
	d.device.UnmapTransferBuffer(transfer)

	cmdbuf, err := d.device.AcquireCommandBuffer()
	if err != nil {
		d.device.ReleaseTexture(tex)
		return nil, fmt.Errorf("render: acquire texture upload command buffer: %w", err)
	}
	copyPass := cmdbuf.BeginCopyPass()
	copyPass.UploadToGPUTexture(
		&sdl.GPUTextureTransferInfo{TransferBuffer: transfer},
		&sdl.GPUTextureRegion{Texture: tex, W: uint32(width), H: uint32(height), D: 1},
		false,
	)
	copyPass.End()
	if err := cmdbuf.Submit(); err != nil {
		return nil, fmt.Errorf("render: submit texture upload: %w", err)
	}
	return tex, nil
}

// NewTextures uploads a same-index-order set of decoded images to the GPU
// (a CMP's texture pages -- scenery, sky, or a craft model's), skipping any
// index whose image is nil (an undecodable CMP member) or that fails
// upload, so the returned slice stays index-aligned with the source even
// when incomplete.
func (d *Device) NewTextures(images []*assets.Image) []*sdl.GPUTexture {
	textures := make([]*sdl.GPUTexture, len(images))
	for i, img := range images {
		if img == nil {
			continue
		}
		tex, err := d.NewTexture(int(img.Width), int(img.Height), img.Pixels)
		if err != nil {
			continue
		}
		textures[i] = tex
	}
	return textures
}

// BeginFrame acquires a command buffer and the swapchain texture this frame
// will eventually present to, and returns an empty triangle accumulator.
func (d *Device) BeginFrame() (*Frame, error) {
	cmdbuf, err := d.device.AcquireCommandBuffer()
	if err != nil {
		return nil, fmt.Errorf("render: acquire command buffer: %w", err)
	}
	swapchain, err := cmdbuf.WaitAndAcquireGPUSwapchainTexture(d.window)
	if err != nil {
		return nil, fmt.Errorf("render: acquire swapchain texture: %w", err)
	}
	return &Frame{cmdbuf: cmdbuf, swapchain: swapchain, byTexture: make(map[*sdl.GPUTexture][]Vertex)}, nil
}

// Submit queues one triangle for presentation. texture may be nil for a
// flat-colored (untextured) triangle, which draws with the device's white
// fallback -- the same visual result as the old renderer's nil-texture
// RenderGeometry path.
func (f *Frame) Submit(triangle [3]Vertex, texture *sdl.GPUTexture) {
	f.byTexture[texture] = append(f.byTexture[texture], triangle[0], triangle[1], triangle[2])
}

// Present draws every triangle accumulated this frame into the offscreen
// color+depth targets (one draw call per distinct texture, real per-pixel
// depth test -- draw order across groups no longer matters, replacing the
// painter's-algorithm centroid sort the old renderer needed), blits the
// result to the swapchain, and submits.
func (d *Device) Present(f *Frame) error {
	if f.swapchain == nil {
		// Minimized or otherwise no swapchain image was available this frame.
		return f.cmdbuf.Cancel()
	}

	offset, starts, err := d.uploadFrameVertices(f)
	if err != nil {
		return err
	}

	renderPass := f.cmdbuf.BeginRenderPass(
		[]sdl.GPUColorTargetInfo{{
			Texture:    d.colorTexture,
			ClearColor: sdl.FColor{R: 10.0 / 255, G: 10.0 / 255, B: 20.0 / 255, A: 1},
			LoadOp:     sdl.GPU_LOADOP_CLEAR,
			StoreOp:    sdl.GPU_STOREOP_STORE,
		}},
		&sdl.GPUDepthStencilTargetInfo{
			Texture:        d.depthTexture,
			ClearDepth:     1,
			LoadOp:         sdl.GPU_LOADOP_CLEAR,
			StoreOp:        sdl.GPU_STOREOP_DONT_CARE,
			StencilLoadOp:  sdl.GPU_LOADOP_DONT_CARE,
			StencilStoreOp: sdl.GPU_STOREOP_DONT_CARE,
		},
	)
	if offset > 0 {
		renderPass.BindGraphicsPipeline(d.pipeline)
		renderPass.BindVertexBuffers([]sdl.GPUBufferBinding{{Buffer: d.vertexBuffer}})
		for texture, verts := range f.byTexture {
			count := len(verts)
			if start, ok := starts[texture]; ok && count > 0 {
				bound := texture
				if bound == nil {
					bound = d.whiteTexture
				}
				renderPass.BindFragmentSamplers([]sdl.GPUTextureSamplerBinding{{Texture: bound, Sampler: d.sampler}})
				renderPass.DrawPrimitives(uint32(count), 1, uint32(start), 0)
			}
		}
	}
	renderPass.End()

	f.cmdbuf.BlitGPUTexture(&sdl.GPUBlitInfo{
		Source:      sdl.GPUBlitRegion{Texture: d.colorTexture, W: uint32(d.width), H: uint32(d.height)},
		Destination: sdl.GPUBlitRegion{Texture: f.swapchain.Texture, W: f.swapchain.Width, H: f.swapchain.Height},
		LoadOp:      sdl.GPU_LOADOP_DONT_CARE,
		Filter:      sdl.GPU_FILTER_NEAREST,
	})

	return f.cmdbuf.Submit()
}

// uploadFrameVertices flattens f's per-texture triangle groups into the
// shared vertex buffer and returns each group's starting vertex offset
// (truncating rather than corrupting the buffer if maxFrameVertices is ever
// exceeded).
func (d *Device) uploadFrameVertices(f *Frame) (int, map[*sdl.GPUTexture]int, error) {
	starts := make(map[*sdl.GPUTexture]int, len(f.byTexture))
	total := 0
	for _, verts := range f.byTexture {
		total += len(verts)
	}
	if total == 0 {
		return 0, starts, nil
	}

	ptr, err := d.device.MapTransferBuffer(d.transferBuffer, true)
	if err != nil {
		return 0, nil, fmt.Errorf("render: map vertex transfer buffer: %w", err)
	}
	dst := unsafe.Slice((*Vertex)(unsafe.Pointer(ptr)), maxFrameVertices)
	offset := 0
	for texture, verts := range f.byTexture {
		starts[texture] = offset
		for _, v := range verts {
			if offset >= maxFrameVertices {
				break
			}
			dst[offset] = v
			offset++
		}
	}
	d.device.UnmapTransferBuffer(d.transferBuffer)

	copyPass := f.cmdbuf.BeginCopyPass()
	copyPass.UploadToGPUBuffer(
		&sdl.GPUTransferBufferLocation{TransferBuffer: d.transferBuffer},
		&sdl.GPUBufferRegion{Buffer: d.vertexBuffer, Size: uint32(offset) * uint32(unsafe.Sizeof(Vertex{}))},
		true,
	)
	copyPass.End()
	return offset, starts, nil
}

// CapturePNG reads back the offscreen color texture (the same one every
// frame draws into, independent of swapchain timing) and PNG-encodes it.
// Safe to call any time after a frame has been presented at least once.
func (d *Device) CapturePNG() ([]byte, error) {
	size := uint32(d.width) * uint32(d.height) * 4
	download, err := d.device.CreateTransferBuffer(&sdl.GPUTransferBufferCreateInfo{
		Usage: sdl.GPU_TRANSFERBUFFERUSAGE_DOWNLOAD, Size: size,
	})
	if err != nil {
		return nil, fmt.Errorf("render: create readback transfer buffer: %w", err)
	}
	defer d.device.ReleaseTransferBuffer(download)

	cmdbuf, err := d.device.AcquireCommandBuffer()
	if err != nil {
		return nil, fmt.Errorf("render: acquire readback command buffer: %w", err)
	}
	copyPass := cmdbuf.BeginCopyPass()
	copyPass.DownloadFromGPUTexture(
		&sdl.GPUTextureRegion{Texture: d.colorTexture, W: uint32(d.width), H: uint32(d.height), D: 1},
		&sdl.GPUTextureTransferInfo{TransferBuffer: download},
	)
	copyPass.End()

	fence, err := cmdbuf.SubmitAndAcquireFence()
	if err != nil {
		return nil, fmt.Errorf("render: submit readback: %w", err)
	}
	defer d.device.ReleaseFence(fence)
	if err := d.device.WaitForFences(true, []*sdl.GPUFence{fence}); err != nil {
		return nil, fmt.Errorf("render: wait for readback: %w", err)
	}

	ptr, err := d.device.MapTransferBuffer(download, false)
	if err != nil {
		return nil, fmt.Errorf("render: map readback transfer buffer: %w", err)
	}
	pixels := unsafe.Slice((*byte)(unsafe.Pointer(ptr)), size)

	frame := image.NewRGBA(image.Rect(0, 0, int(d.width), int(d.height)))
	copy(frame.Pix, pixels)
	d.device.UnmapTransferBuffer(download)

	var encoded bytes.Buffer
	if err := png.Encode(&encoded, frame); err != nil {
		return nil, fmt.Errorf("render: encode captured frame: %w", err)
	}
	return encoded.Bytes(), nil
}

// ReleaseTexture releases a texture created by NewTexture (e.g. a track tile
// texture owned by TrackRenderer, released when the track is torn down).
func (d *Device) ReleaseTexture(tex *sdl.GPUTexture) {
	if d == nil || d.device == nil || tex == nil {
		return
	}
	d.device.ReleaseTexture(tex)
}

// submitScreenTriangle projects three already-camera-space vertices to
// screen space with the shared PS1 GTE-derived focal distances and -- if
// front-facing or cull is false -- submits it to frame in NDC with
// texture/color from the caller.
//
// cull should be true only for genuinely closed, convex-ish meshes viewed
// from outside (the ship hull): a single fixed winding sign is the right
// tool there. Track/scenery/sky geometry is a tunnel viewed from *inside*,
// where up-facing floor and down-facing ceiling/wall quads both need to stay
// visible to the same camera -- applying the same cull sign to those
// discarded real, legitimately-visible faces (confirmed by temporarily
// disabling culling entirely and seeing previously-missing tunnel/wall
// structure appear), which is why those callers pass cull=false.
func submitScreenTriangle(frame *Frame, corners [3]perspectiveVertex, focalX, focalY, width, height float32, texture *sdl.GPUTexture, color sdl.FColor, cull bool) {
	var screen [3]sdl.FPoint
	for i, corner := range corners {
		screen[i] = sdl.FPoint{
			X: width/2 + corner.position.X*focalX/corner.position.Z,
			Y: height/2 + corner.position.Y*focalY/corner.position.Z,
		}
	}
	if cull && backFacing(screen[0], screen[1], screen[2]) {
		return
	}

	var triangle [3]Vertex
	for i, corner := range corners {
		triangle[i] = Vertex{
			X: screen[i].X/width*2 - 1,
			Y: 1 - screen[i].Y/height*2,
			Z: corner.position.Z,
			U: corner.uv.X, V: corner.uv.Y,
			R: color.R, G: color.G, B: color.B, A: color.A,
		}
	}
	frame.Submit(triangle, texture)
}

// backFacing reports whether a screen-space triangle winds away from the
// viewer, using the signed area of its projected 2D vertices. PS1 PRM/TRV
// data is authored with a fixed winding convention; this is a standard
// screen-space backface test, independent of any 3D normal convention.
func backFacing(a, b, c sdl.FPoint) bool {
	area := (b.X-a.X)*(c.Y-a.Y) - (c.X-a.X)*(b.Y-a.Y)
	return area > 0
}

// Destroy releases every GPU resource this Device owns, in dependency order,
// then releases the window and destroys the device itself.
func (d *Device) Destroy() {
	if d == nil || d.device == nil {
		return
	}
	if d.pipeline != nil {
		d.device.ReleaseGraphicsPipeline(d.pipeline)
	}
	if d.vertexBuffer != nil {
		d.device.ReleaseBuffer(d.vertexBuffer)
	}
	if d.transferBuffer != nil {
		d.device.ReleaseTransferBuffer(d.transferBuffer)
	}
	if d.sampler != nil {
		d.device.ReleaseSampler(d.sampler)
	}
	if d.whiteTexture != nil {
		d.device.ReleaseTexture(d.whiteTexture)
	}
	if d.colorTexture != nil {
		d.device.ReleaseTexture(d.colorTexture)
	}
	if d.depthTexture != nil {
		d.device.ReleaseTexture(d.depthTexture)
	}
	if d.window != nil {
		d.device.ReleaseWindow(d.window)
	}
	d.device.Destroy()
}
