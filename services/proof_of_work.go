package services

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"math/rand"
	"regexp"
	"sync"
	"time"

	"github.com/google/uuid"
	"golang.org/x/crypto/sha3"
)

var (
	powCores = []int{8, 16, 24, 32}

	cachedScripts      []string
	cachedDpl          string
	cachedTime         int64
	powMu sync.Mutex

	navigatorKey = []string{
		"registerProtocolHandler−function registerProtocolHandler() { [native code] }",
		"storage−[object StorageManager]",
		"locks−[object LockManager]",
		"appCodeName−Mozilla",
		"permissions−[object Permissions]",
		"share−function share() { [native code] }",
		"webdriver−false",
		"managed−[object NavigatorManagedData]",
		"canShare−function canShare() { [native code] }",
		"vendor−Google Inc.",
		"vendor−Google Inc.",
		"mediaDevices−[object MediaDevices]",
		"vibrate−function vibrate() { [native code] }",
		"storageBuckets−[object StorageBucketManager]",
		"mediaCapabilities−[object MediaCapabilities]",
		"getGamepads−function getGamepads() { [native code] }",
		"bluetooth−[object Bluetooth]",
		"share−function share() { [native code] }",
		"cookieEnabled−true",
		"virtualKeyboard−[object VirtualKeyboard]",
		"product−Gecko",
		"mediaDevices−[object MediaDevices]",
		"canShare−function canShare() { [native code] }",
		"getGamepads−function getGamepads() { [native code] }",
		"product−Gecko",
		"xr−[object XRSystem]",
		"clipboard−[object Clipboard]",
		"storageBuckets−[object StorageBucketManager]",
		"unregisterProtocolHandler−function unregisterProtocolHandler() { [native code] }",
		"productSub−20030107",
		"login−[object NavigatorLogin]",
		"vendorSub−",
		"login−[object NavigatorLogin]",
		"getInstalledRelatedApps−function getInstalledRelatedApps() { [native code] }",
		"mediaDevices−[object MediaDevices]",
		"locks−[object LockManager]",
		"webkitGetUserMedia−function webkitGetUserMedia() { [native code] }",
		"vendor−Google Inc.",
		"xr−[object XRSystem]",
		"mediaDevices−[object MediaDevices]",
		"virtualKeyboard−[object VirtualKeyboard]",
		"virtualKeyboard−[object VirtualKeyboard]",
		"appName−Netscape",
		"storageBuckets−[object StorageBucketManager]",
		"presentation−[object Presentation]",
		"onLine−true",
		"mimeTypes−[object MimeTypeArray]",
		"credentials−[object CredentialsContainer]",
		"presentation−[object Presentation]",
		"getGamepads−function getGamepads() { [native code] }",
		"vendorSub−",
		"virtualKeyboard−[object VirtualKeyboard]",
		"serviceWorker−[object ServiceWorkerContainer]",
		"xr−[object XRSystem]",
		"product−Gecko",
		"keyboard−[object Keyboard]",
		"gpu−[object GPU]",
		"getInstalledRelatedApps−function getInstalledRelatedApps() { [native code] }",
		"webkitPersistentStorage−[object DeprecatedStorageQuota]",
		"doNotTrack",
		"clearAppBadge−function clearAppBadge() { [native code] }",
		"presentation−[object Presentation]",
		"serial−[object Serial]",
		"locks−[object LockManager]",
		"requestMIDIAccess−function requestMIDIAccess() { [native code] }",
		"locks−[object LockManager]",
		"requestMediaKeySystemAccess−function requestMediaKeySystemAccess() { [native code] }",
		"vendor−Google Inc.",
		"pdfViewerEnabled−true",
		"language−zh-CN",
		"setAppBadge−function setAppBadge() { [native code] }",
		"geolocation−[object Geolocation]",
		"userAgentData−[object NavigatorUAData]",
		"mediaCapabilities−[object MediaCapabilities]",
		"requestMIDIAccess−function requestMIDIAccess() { [native code] }",
		"getUserMedia−function getUserMedia() { [native code] }",
		"mediaDevices−[object MediaDevices]",
		"webkitPersistentStorage−[object DeprecatedStorageQuota]",
		"sendBeacon−function sendBeacon() { [native code] }",
		"hardwareConcurrency−32",
		"credentials−[object CredentialsContainer]",
		"storage−[object StorageManager]",
		"cookieEnabled−true",
		"pdfViewerEnabled−true",
		"windowControlsOverlay−[object WindowControlsOverlay]",
		"scheduling−[object Scheduling]",
		"pdfViewerEnabled−true",
		"hardwareConcurrency−32",
		"xr−[object XRSystem]",
		"webdriver−false",
		"getInstalledRelatedApps−function getInstalledRelatedApps() { [native code] }",
		"getInstalledRelatedApps−function getInstalledRelatedApps() { [native code] }",
		"bluetooth−[object Bluetooth]",
	}
	documentKey = []string{"_reactListeningo743lnnpvdg", "location"}
	windowKey   = []string{
		"0", "window", "self", "document", "name", "location", "customElements",
		"history", "navigation", "locationbar", "menubar", "personalbar", "scrollbars",
		"statusbar", "toolbar", "status", "closed", "frames", "length", "top", "opener",
		"parent", "frameElement", "navigator", "origin", "external", "screen", "innerWidth",
		"innerHeight", "scrollX", "pageXOffset", "scrollY", "pageYOffset", "visualViewport",
		"screenX", "screenY", "outerWidth", "outerHeight", "devicePixelRatio",
		"clientInformation", "screenLeft", "screenTop", "styleMedia", "onsearch",
		"isSecureContext", "trustedTypes", "performance", "onappinstalled",
		"onbeforeinstallprompt", "crypto", "indexedDB", "sessionStorage", "localStorage",
		"onbeforexrselect", "onabort", "onbeforeinput", "onbeforematch", "onbeforetoggle",
		"onblur", "oncancel", "oncanplay", "oncanplaythrough", "onchange", "onclick",
		"onclose", "oncontentvisibilityautostatechange", "oncontextlost", "oncontextmenu",
		"oncontextrestored", "oncuechange", "ondblclick", "ondrag", "ondragend",
		"ondragenter", "ondragleave", "ondragover", "ondragstart", "ondrop",
		"ondurationchange", "onemptied", "onended", "onerror", "onfocus", "onformdata",
		"oninput", "oninvalid", "onkeydown", "onkeypress", "onkeyup", "onload",
		"onloadeddata", "onloadedmetadata", "onloadstart", "onmousedown", "onmouseenter",
		"onmouseleave", "onmousemove", "onmouseout", "onmouseover", "onmouseup",
		"onmousewheel", "onpause", "onplay", "onplaying", "onprogress", "onratechange",
		"onreset", "onresize", "onscroll", "onsecuritypolicyviolation", "onseeked",
		"onseeking", "onselect", "onslotchange", "onstalled", "onsubmit", "onsuspend",
		"ontimeupdate", "ontoggle", "onvolumechange", "onwaiting",
		"onwebkitanimationend", "onwebkitanimationiteration", "onwebkitanimationstart",
		"onwebkittransitionend", "onwheel", "onauxclick", "ongotpointercapture",
		"onlostpointercapture", "onpointerdown", "onpointermove", "onpointerrawupdate",
		"onpointerup", "onpointercancel", "onpointerover", "onpointerout",
		"onpointerenter", "onpointerleave", "onselectstart", "onselectionchange",
		"onanimationend", "onanimationiteration", "onanimationstart", "ontransitionrun",
		"ontransitionstart", "ontransitionend", "ontransitioncancel", "onafterprint",
		"onbeforeprint", "onbeforeunload", "onhashchange", "onlanguagechange",
		"onmessage", "onmessageerror", "onoffline", "ononline", "onpagehide",
		"onpageshow", "onpopstate", "onrejectionhandled", "onstorage",
		"onunhandledrejection", "onunload", "crossOriginIsolated", "scheduler", "alert",
		"atob", "blur", "btoa", "cancelAnimationFrame", "cancelIdleCallback",
		"captureEvents", "clearInterval", "clearTimeout", "close", "confirm",
		"createImageBitmap", "fetch", "find", "focus", "getComputedStyle", "getSelection",
		"matchMedia", "moveBy", "moveTo", "open", "postMessage", "print", "prompt",
		"queueMicrotask", "releaseEvents", "reportError", "requestAnimationFrame",
		"requestIdleCallback", "resizeBy", "resizeTo", "scroll", "scrollBy", "scrollTo",
		"setInterval", "setTimeout", "stop", "structuredClone",
		"webkitCancelAnimationFrame", "webkitRequestAnimationFrame", "chrome", "caches",
		"cookieStore", "ondevicemotion", "ondeviceorientation",
		"ondeviceorientationabsolute", "launchQueue", "documentPictureInPicture",
		"getScreenDetails", "queryLocalFonts", "showDirectoryPicker", "showOpenFilePicker",
		"showSaveFilePicker", "originAgentCluster", "onpageswap", "onpagereveal",
		"credentialless", "speechSynthesis", "onscrollend", "webkitRequestFileSystem",
		"webkitResolveLocalFileSystemURL", "sendMsgToSolverCS", "webpackChunk_N_E",
		"__next_set_public_path__", "next", "__NEXT_DATA__", "__SSG_MANIFEST_CB",
		"__NEXT_P", "_N_E", "regeneratorRuntime", "__REACT_INTL_CONTEXT__", "DD_RUM", "_",
		"filterCSS", "filterXSS", "__SEGMENT_INSPECTOR__", "__NEXT_PRELOADREADY",
		"Intercom", "__MIDDLEWARE_MATCHERS", "__STATSIG_SDK__", "__STATSIG_JS_SDK__",
		"__STATSIG_RERENDER_OVERRIDE__", "_oaiHandleSessionExpired", "__BUILD_MANIFEST",
		"__SSG_MANIFEST", "__intercomAssignLocation", "__intercomReloadLocation",
	}
)

func GetDataBuildFromHTML(htmlContent string) {
	powMu.Lock()
	defer powMu.Unlock()

	re := regexp.MustCompile(`<script[^>]+src="([^"]+)"`)
	matches := re.FindAllStringSubmatch(htmlContent, -1)
	for _, m := range matches {
		if len(m) > 1 {
			cachedScripts = append(cachedScripts, m[1])
			dplRe := regexp.MustCompile(`c/[^/]*/_`)
			if dm := dplRe.FindString(m[1]); dm != "" {
				cachedDpl = dm
				cachedTime = time.Now().Unix()
			}
		}
	}

	if len(cachedScripts) == 0 {
		cachedScripts = append(cachedScripts, "https://chatgpt.com/backend-api/sentinel/sdk.js")
	}
	if cachedDpl == "" {
		buildRe := regexp.MustCompile(`<html[^>]*data-build="([^"]*)"`)
		if bm := buildRe.FindStringSubmatch(htmlContent); len(bm) > 1 {
			cachedDpl = bm[1]
			cachedTime = time.Now().Unix()
		}
	}
}

func getParseTime() string {
	loc := time.FixedZone("EST", -5*3600)
	now := time.Now().In(loc)
	return now.Format("Mon Jan 02 2006 15:04:05") + " GMT-0500 (Eastern Standard Time)"
}

func GetPowConfig(userAgent string) []any {
	powMu.Lock()
	defer powMu.Unlock()

	screenSizes := []int{1920 + 1080, 2560 + 1440, 1920 + 1200, 2560 + 1600}

	var script string
	if len(cachedScripts) > 0 {
		script = cachedScripts[rand.Intn(len(cachedScripts))]
	}

	config := []any{
		screenSizes[rand.Intn(len(screenSizes))],
		getParseTime(),
		4294705152,
		0,
		userAgent,
		script,
		cachedDpl,
		"en-US",
		"en-US,es-US,en,es",
		0,
		navigatorKey[rand.Intn(len(navigatorKey))],
		documentKey[rand.Intn(len(documentKey))],
		windowKey[rand.Intn(len(windowKey))],
		float64(time.Duration(rand.Intn(100000)) * time.Microsecond / time.Millisecond),
		uuid.New().String(),
		"",
		powCores[rand.Intn(len(powCores))],
		float64(time.Now().UnixMilli()) - float64(time.Duration(rand.Intn(100000))*time.Microsecond/time.Millisecond),
	}
	return config
}

func generateAnswer(seed, diff string, config []any) (string, bool) {
	diffLen := len(diff) / 2
	seedBytes := []byte(seed)

	part1, _ := json.Marshal(config[:3])
	p1 := string(part1[:len(part1)-1]) + ","

	part2, _ := json.Marshal(config[4:9])
	p2Str := string(part2)
	p2 := "," + p2Str[1:len(p2Str)-1] + ","

	part3, _ := json.Marshal(config[10:])
	p3Str := string(part3)
	p3 := "," + p3Str[1:]

	targetDiff, err := hexToBytes(diff)
	if err != nil {
		fallback := "wQ8Lk5FbGpA2NcR9dShT6gYjU7VxZ4D" + base64.StdEncoding.EncodeToString([]byte(`"`+seed+`"`))
		return fallback, false
	}

	prefix1 := []byte(p1)
	prefix2 := []byte(p2)
	prefix3 := []byte(p3)

	for i := 0; i < 500000; i++ {
		left := []byte(fmt.Sprintf("%d", i))
		right := []byte(fmt.Sprintf("%d", i>>1))

		finalJSON := make([]byte, 0, len(prefix1)+len(left)+len(prefix2)+len(right)+len(prefix3))
		finalJSON = append(finalJSON, prefix1...)
		finalJSON = append(finalJSON, left...)
		finalJSON = append(finalJSON, prefix2...)
		finalJSON = append(finalJSON, right...)
		finalJSON = append(finalJSON, prefix3...)

		encoded := base64.StdEncoding.EncodeToString(finalJSON)
		hash := sha3.Sum512(append(seedBytes, []byte(encoded)...))
		if compareBytes(hash[:diffLen], targetDiff) {
			return encoded, true
		}
	}

	fallback := "wQ8Lk5FbGpA2NcR9dShT6gYjU7VxZ4D" + base64.StdEncoding.EncodeToString([]byte(`"`+seed+`"`))
	return fallback, false
}

func hexToBytes(hex string) ([]byte, error) {
	if len(hex)%2 != 0 {
		hex = "0" + hex
	}
	result := make([]byte, len(hex)/2)
	for i := 0; i < len(hex); i += 2 {
		var b byte
		for j := 0; j < 2; j++ {
			c := hex[i+j]
			switch {
			case c >= '0' && c <= '9':
				b = b*16 + (c - '0')
			case c >= 'a' && c <= 'f':
				b = b*16 + (c - 'a' + 10)
			case c >= 'A' && c <= 'F':
				b = b*16 + (c - 'A' + 10)
			default:
				return nil, fmt.Errorf("invalid hex char: %c", c)
			}
		}
		result[i/2] = b
	}
	return result, nil
}

func compareBytes(a, b []byte) bool {
	for i := 0; i < len(a) && i < len(b); i++ {
		if a[i] < b[i] {
			return true
		}
		if a[i] > b[i] {
			return false
		}
	}
	return true
}

func GetAnswerToken(seed, diff string, config []any) (string, bool) {
	answer, solved := generateAnswer(seed, diff, config)
	return "gAAAAAB" + answer, solved
}

func GetRequirementsToken(config []any) string {
	seed := fmt.Sprintf("%v", rand.Float64())
	answer, _ := generateAnswer(seed, "0fffff", config)
	return "gAAAAAC" + answer
}

func GenerateProofToken(seed, difficulty, userAgent string, proofConfig []any) string {
	if proofConfig == nil {
		proofConfig = GetPowConfig(userAgent)
	}
	answer, _ := GetAnswerToken(seed, difficulty, proofConfig)
	return answer
}

