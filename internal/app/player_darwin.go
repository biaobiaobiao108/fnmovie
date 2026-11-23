package app

import (
	"errors"
	"fmt"
	"math"
	"runtime"
	"sync"
	"time"
	"unsafe"

	"github.com/ebitengine/purego"
	"github.com/ebitengine/purego/objc"
	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
)

const playerMpvFilename = "libmpv.dylib"
const playerResourcePlatform = "darwin-" + runtime.GOARCH

func openMpvLibrary(path string) (func(string) (uintptr, error), func(), error) {
	handle, err := purego.Dlopen(path, purego.RTLD_NOW|purego.RTLD_LOCAL)
	if err != nil {
		return nil, nil, err
	}
	return func(name string) (uintptr, error) { return purego.Dlsym(handle, name) },
		func() { _ = purego.Dlclose(handle) }, nil
}

func readCBytes(address uintptr, size int) ([]byte, bool) {
	if address == 0 || size <= 0 {
		return nil, false
	}
	// The caller consumes a C-owned event or node before libmpv releases it.
	pointer := *(*unsafe.Pointer)(unsafe.Pointer(&address))
	return unsafe.Slice((*byte)(pointer), size), true
}

func cString(address uintptr) string {
	if address == 0 {
		return ""
	}
	pointer := *(*unsafe.Pointer)(unsafe.Pointer(&address))
	size := 0
	for *(*byte)(unsafe.Add(pointer, size)) != 0 {
		size++
	}
	return string(unsafe.Slice((*byte)(pointer), size))
}

func playerMpvHost(surface uintptr) uintptr { return surface }

func playerMpvOptions(uintptr) [][2]string {
	return [][2]string{{"vo", "libmpv"}, {"ao", "coreaudio"}}
}

type cocoaPoint struct{ X, Y float64 }
type cocoaSize struct{ Width, Height float64 }
type cocoaRect struct {
	Origin cocoaPoint
	Size   cocoaSize
}

var (
	playerViewClass      objc.Class
	playerViewClassError error
	playerViewClassOnce  sync.Once
	playerViewActions    = map[uintptr]func(){} // UI thread
)

func createPlayerSurface(parent uintptr) (uintptr, error) {
	content := objc.ID(parent).Send(objc.RegisterName("contentView"))
	if content == 0 {
		return 0, errors.New("找不到播放器窗口的内容视图")
	}
	playerViewClassOnce.Do(func() {
		playerViewClass, playerViewClassError = objc.RegisterClass("FnMovieOpenGLView", objc.GetClass("NSOpenGLView"), nil, nil, []objc.MethodDef{
			{Cmd: objc.RegisterName("mouseDown:"), Fn: func(view objc.ID, _ objc.SEL, event objc.ID) {
				if objc.Send[int64](event, objc.RegisterName("clickCount")) == 2 {
					if action := playerViewActions[uintptr(view)]; action != nil {
						action()
					}
				}
			}},
		})
	})
	if playerViewClassError != nil {
		return 0, playerViewClassError
	}

	// NSOpenGLPFAOpenGLProfile, NSOpenGLProfileVersion3_2Core,
	// NSOpenGLPFAAccelerated, NSOpenGLPFADoubleBuffer.
	attributes := [...]uint32{99, 0x3200, 73, 5, 0}
	format := objc.ID(objc.GetClass("NSOpenGLPixelFormat")).Send(objc.RegisterName("alloc")).
		Send(objc.RegisterName("initWithAttributes:"), &attributes[0])
	if format == 0 {
		return 0, errors.New("创建 macOS OpenGL 像素格式失败")
	}
	defer format.Send(objc.RegisterName("release"))

	bounds := objc.Send[cocoaRect](content, objc.RegisterName("bounds"))
	view := objc.ID(playerViewClass).Send(objc.RegisterName("alloc")).
		Send(objc.RegisterName("initWithFrame:pixelFormat:"), bounds, format)
	if view == 0 {
		return 0, errors.New("创建 macOS 播放视图失败")
	}
	view.Send(objc.RegisterName("setWantsBestResolutionOpenGLSurface:"), true)
	view.Send(objc.RegisterName("setAutoresizingMask:"), uintptr(2|16)) // width and height
	view.Send(objc.RegisterName("setHidden:"), true)
	content.Send(objc.RegisterName("addSubview:"), view)
	return uintptr(view), nil
}

func setPlayerSurfaceDoubleClick(surface uintptr, action func()) {
	playerViewActions[surface] = action
}

func showPlayerSurface(surface uintptr) {
	objc.ID(surface).Send(objc.RegisterName("setHidden:"), false)
}

func hidePlayerSurface(surface uintptr) {
	objc.ID(surface).Send(objc.RegisterName("setHidden:"), true)
}

func destroyPlayerSurface(surface uintptr) {
	delete(playerViewActions, surface)
	view := objc.ID(surface)
	view.Send(objc.RegisterName("removeFromSuperview"))
	view.Send(objc.RegisterName("release"))
}

func windowScale(handle uintptr) float64 {
	return max(1, objc.Send[float64](objc.ID(handle), objc.RegisterName("backingScaleFactor")))
}

func setPlayerSurfaceViewport(uintptr, ui.Rect)            {} // AppKit autoresizes the subview.
func setupPlayerHeaderWindowDrag(uintptr, uintptr, func()) {}
func cleanupPlayerHeaderWindowDrag(uintptr)                {}
func findMyGoCaptionButtons(uintptr) uintptr               { return 0 }
func placePlayerOverlayAboveSurface(uintptr, uintptr)      {} // Native MyGo child windows sit above the parent.
func updatePlayerOverlayShapes(uintptr, uintptr, uintptr)  {}

func watchPlayerPointer(done <-chan struct{}, surface uintptr, activity, finished chan<- struct{}) {
	defer close(finished)
	ticker := time.NewTicker(35 * time.Millisecond)
	defer ticker.Stop()
	var previous cocoaPoint
	for {
		select {
		case <-done:
			return
		case <-ticker.C:
		}
		mygo.RunOnMain(func() {
			select {
			case <-done:
				return
			default:
			}
			view := objc.ID(surface)
			window := view.Send(objc.RegisterName("window"))
			if window == 0 {
				return
			}
			mouse := objc.Send[cocoaPoint](window, objc.RegisterName("mouseLocationOutsideOfEventStream"))
			if mouse == previous {
				return
			}
			previous = mouse
			mouse = objc.Send[cocoaPoint](view, objc.RegisterName("convertPoint:fromView:"), mouse, objc.ID(0))
			bounds := objc.Send[cocoaRect](view, objc.RegisterName("bounds"))
			if mouse.X >= 0 && mouse.X < bounds.Size.Width && mouse.Y >= 0 && mouse.Y < bounds.Size.Height {
				select {
				case activity <- struct{}{}:
				default:
				}
			}
		})
	}
}

type mpvRenderParam struct {
	kind int32
	data unsafe.Pointer
}

type mpvGLInitParams struct {
	getProcAddress uintptr
	context        uintptr
}

type mpvGLFramebuffer struct {
	id, width, height, format int32
}

var darwinRenderAPI struct {
	once     sync.Once
	err      error
	create   func(*uintptr, uintptr, *mpvRenderParam) int32
	callback func(uintptr, uintptr, uintptr)
	update   func(uintptr) uint64
	render   func(uintptr, *mpvRenderParam) int32
	free     func(uintptr)
	getProc  uintptr
	wakeup   uintptr
}

var renderWakeups sync.Map // libmpv render handle to buffered channel

func loadDarwinRenderAPI(path string) error {
	api := &darwinRenderAPI
	api.once.Do(func() {
		lookup, closeMpv, err := openMpvLibrary(path)
		if err != nil {
			api.err = err
			return
		}
		for _, symbol := range []struct {
			name string
			fn   any
		}{
			{"mpv_render_context_create", &api.create}, {"mpv_render_context_set_update_callback", &api.callback},
			{"mpv_render_context_update", &api.update}, {"mpv_render_context_render", &api.render},
			{"mpv_render_context_free", &api.free},
		} {
			address, err := lookup(symbol.name)
			if err != nil {
				closeMpv()
				api.err = err
				return
			}
			purego.RegisterFunc(symbol.fn, address)
		}
		gl, err := purego.Dlopen("/System/Library/Frameworks/OpenGL.framework/OpenGL", purego.RTLD_NOW|purego.RTLD_LOCAL)
		if err != nil {
			closeMpv()
			api.err = err
			return
		}
		api.getProc = purego.NewCallback(func(_ uintptr, name uintptr) uintptr {
			address, _ := purego.Dlsym(gl, cString(name))
			return address
		})
		api.wakeup = purego.NewCallback(func(context uintptr) {
			if value, ok := renderWakeups.Load(context); ok {
				select {
				case value.(chan struct{}) <- struct{}{}:
				default:
				}
			}
		})
	})
	return api.err
}

func startMpvRenderer(path string, mpv, surface uintptr) (func(), error) {
	if err := loadDarwinRenderAPI(path); err != nil {
		return nil, fmt.Errorf("加载 macOS libmpv 渲染接口失败：%w", err)
	}
	api := &darwinRenderAPI
	view := objc.ID(surface)
	var context objc.ID
	var render uintptr
	var code int32
	updates := make(chan struct{}, 1)
	done := make(chan struct{})
	mygo.RunOnMain(func() {
		context = view.Send(objc.RegisterName("openGLContext"))
		if context == 0 {
			return
		}
		context.Send(objc.RegisterName("makeCurrentContext"))
		defer objc.ID(objc.GetClass("NSOpenGLContext")).Send(objc.RegisterName("clearCurrentContext"))
		gl := mpvGLInitParams{getProcAddress: api.getProc}
		name := [...]byte{'o', 'p', 'e', 'n', 'g', 'l', 0}
		params := [...]mpvRenderParam{
			{kind: 1, data: unsafe.Pointer(&name[0])}, // API_TYPE
			{kind: 2, data: unsafe.Pointer(&gl)},      // OPENGL_INIT_PARAMS
			{},
		}
		code = api.create(&render, mpv, &params[0])
		if code >= 0 && render != 0 {
			renderWakeups.Store(render, updates)
			api.callback(render, api.wakeup, render)
		}
	})
	if render == 0 || code < 0 {
		return nil, fmt.Errorf("创建 macOS libmpv 渲染上下文失败（%d）", code)
	}

	go func() {
		// AppKit handles drawable changes, including backing-scale changes that
		// can occur while playback is paused and no new frame is queued.
		ticker := time.NewTicker(80 * time.Millisecond)
		defer ticker.Stop()
		var lastWidth, lastHeight int32
		for {
			select {
			case <-done:
				return
			case <-updates:
			case <-ticker.C:
			}
			mygo.RunOnMain(func() {
				select {
				case <-done:
					return
				default:
				}
				if objc.Send[bool](view, objc.RegisterName("isHidden")) {
					lastWidth, lastHeight = 0, 0
					return
				}
				bounds := objc.Send[cocoaRect](view, objc.RegisterName("bounds"))
				backing := objc.Send[cocoaRect](view, objc.RegisterName("convertRectToBacking:"), bounds)
				width, height := int32(math.Round(backing.Size.Width)), int32(math.Round(backing.Size.Height))
				if width <= 0 || height <= 0 {
					return
				}
				context.Send(objc.RegisterName("makeCurrentContext"))
				defer objc.ID(objc.GetClass("NSOpenGLContext")).Send(objc.RegisterName("clearCurrentContext"))
				changed := width != lastWidth || height != lastHeight
				if api.update(render)&1 == 0 && !changed { // MPV_RENDER_UPDATE_FRAME
					return
				}
				if changed {
					context.Send(objc.RegisterName("update"))
				}
				fbo := mpvGLFramebuffer{width: width, height: height}
				flip := int32(1)
				params := [...]mpvRenderParam{
					{kind: 3, data: unsafe.Pointer(&fbo)},  // OPENGL_FBO
					{kind: 4, data: unsafe.Pointer(&flip)}, // FLIP_Y
					{},
				}
				if api.render(render, &params[0]) >= 0 {
					context.Send(objc.RegisterName("flushBuffer"))
					lastWidth, lastHeight = width, height
				}
			})
		}
	}()
	return func() {
		close(done)
		mygo.RunOnMain(func() {
			api.callback(render, 0, 0)
			renderWakeups.Delete(render)
			context.Send(objc.RegisterName("makeCurrentContext"))
			api.free(render)
			objc.ID(objc.GetClass("NSOpenGLContext")).Send(objc.RegisterName("clearCurrentContext"))
		})
	}, nil
}
