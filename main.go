package main

import (
	"bufio"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"net/http"
	"strconv"
	"strings"
	"sync"

	"github.com/gorilla/websocket"
	"go.bug.st/serial"
)

// Quaternion represents a quaternion with i, j, k, real components
type Quaternion struct {
	I    float64 `json:"i"`
	J    float64 `json:"j"`
	K    float64 `json:"k"`
	Real float64 `json:"real"`
}

var (
	currentQuat  Quaternion
	quatMutex    sync.RWMutex
	clients      = make(map[*websocket.Conn]bool)
	clientsMutex sync.Mutex
	upgrader     = websocket.Upgrader{
		CheckOrigin: func(r *http.Request) bool {
			return true // Allow all origins for simplicity
		},
	}
	portName = flag.String("port", "COM3", "Serial port name (e.g., COM3 on Windows, /dev/ttyUSB0 on Linux)")
	baudRate = flag.Int("baud", 115200, "Baud rate for serial port")
	webPort  = flag.String("web", "8080", "HTTP server port")
)

func main() {
	flag.Parse()

	// Start serial port listener
	go listenSerialPort()

	// Setup HTTP server
	http.HandleFunc("/", serveHome)
	http.HandleFunc("/ws", handleWebSocket)

	addr := fmt.Sprintf(":%s", *webPort)
	log.Printf("Starting web server on http://localhost%s", addr)
	log.Printf("Listening to serial port: %s at %d baud", *portName, *baudRate)

	if err := http.ListenAndServe(addr, nil); err != nil {
		log.Fatal("ListenAndServe error:", err)
	}
}

// listenSerialPort reads quaternion data from the serial port
func listenSerialPort() {
	mode := &serial.Mode{
		BaudRate: *baudRate,
	}

	for {
		port, err := serial.Open(*portName, mode)
		if err != nil {
			log.Printf("Error opening serial port %s: %v. Retrying in 5 seconds...", *portName, err)
			// Wait and retry
			continue
		}

		log.Printf("Successfully opened serial port: %s", *portName)
		scanner := bufio.NewScanner(port)

		for scanner.Scan() {
			line := scanner.Text()
			quat, err := parseQuaternion(line)
			if err != nil {
				log.Printf("Error parsing quaternion: %v (line: %s)", err, line)
				continue
			}

			// Update current quaternion
			quatMutex.Lock()
			currentQuat = quat
			quatMutex.Unlock()

			// Broadcast to all connected clients
			broadcastQuaternion(quat)
		}

		if err := scanner.Err(); err != nil {
			log.Printf("Error reading from serial port: %v", err)
		}

		port.Close()
		log.Println("Serial port closed. Reconnecting...")
	}
}

// parseQuaternion parses a line in format "i,j,k,real"
func parseQuaternion(line string) (Quaternion, error) {
	parts := strings.Split(strings.TrimSpace(line), ",")
	if len(parts) != 4 {
		return Quaternion{}, fmt.Errorf("expected 4 values, got %d", len(parts))
	}

	i, err := strconv.ParseFloat(parts[0], 64)
	if err != nil {
		return Quaternion{}, fmt.Errorf("invalid i value: %v", err)
	}

	j, err := strconv.ParseFloat(parts[1], 64)
	if err != nil {
		return Quaternion{}, fmt.Errorf("invalid j value: %v", err)
	}

	k, err := strconv.ParseFloat(parts[2], 64)
	if err != nil {
		return Quaternion{}, fmt.Errorf("invalid k value: %v", err)
	}

	real, err := strconv.ParseFloat(parts[3], 64)
	if err != nil {
		return Quaternion{}, fmt.Errorf("invalid real value: %v", err)
	}

	return Quaternion{I: i, J: j, K: k, Real: real}, nil
}

// broadcastQuaternion sends quaternion data to all connected WebSocket clients
func broadcastQuaternion(quat Quaternion) {
	clientsMutex.Lock()
	defer clientsMutex.Unlock()

	data, err := json.Marshal(quat)
	if err != nil {
		log.Printf("Error marshaling quaternion: %v", err)
		return
	}

	for client := range clients {
		err := client.WriteMessage(websocket.TextMessage, data)
		if err != nil {
			log.Printf("WebSocket write error: %v", err)
			client.Close()
			delete(clients, client)
		}
	}
}

// handleWebSocket handles WebSocket connections
func handleWebSocket(w http.ResponseWriter, r *http.Request) {
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Printf("WebSocket upgrade error: %v", err)
		return
	}

	clientsMutex.Lock()
	clients[conn] = true
	clientsMutex.Unlock()

	log.Println("New WebSocket client connected")

	// Send current quaternion immediately
	quatMutex.RLock()
	quat := currentQuat
	quatMutex.RUnlock()

	data, _ := json.Marshal(quat)
	conn.WriteMessage(websocket.TextMessage, data)

	// Keep connection alive and handle disconnection
	defer func() {
		clientsMutex.Lock()
		delete(clients, conn)
		clientsMutex.Unlock()
		conn.Close()
		log.Println("WebSocket client disconnected")
	}()

	// Read messages from client (for keep-alive)
	for {
		_, _, err := conn.ReadMessage()
		if err != nil {
			break
		}
	}
}

// serveHome serves the main HTML page
func serveHome(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Content-Type", "text/html")
	w.Write([]byte(htmlContent))
}

const htmlContent = `<!DOCTYPE html>
<html lang="en">
<head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <title>Quaternion 3D Viewer</title>
    <style>
        body {
            margin: 0;
            padding: 0;
            font-family: Arial, sans-serif;
            overflow: hidden;
            background: linear-gradient(135deg, #667eea 0%, #764ba2 100%);
        }
        #container {
            width: 100vw;
            height: 100vh;
            display: flex;
            flex-direction: column;
            position: relative;
        }
        #topBar {
            background: transparent;
            padding: 10px 15px;
            display: flex;
            justify-content: space-between;
            align-items: center;
            z-index: 100;
            position: absolute;
            top: 0;
            left: 0;
            right: 0;
        }
        #hamburger {
            cursor: pointer;
            padding: 8px 12px;
            user-select: none;
            z-index: 102;
            background: rgba(0, 0, 0, 0.5);
            border-radius: 5px;
            transition: background 0.3s, box-shadow 0.3s;
            display: flex;
            flex-direction: column;
            gap: 4px;
            width: 30px;
            height: 30px;
            justify-content: center;
            align-items: center;
        }
        #hamburger span {
            width: 20px;
            height: 2px;
            background: white;
            border-radius: 1px;
            transition: all 0.3s;
        }
        #hamburger:hover {
            background: rgba(0, 0, 0, 0.7);
            box-shadow: 0 2px 8px rgba(0,0,0,0.3);
        }
        #infoToggle {
            font-size: 20px;
            cursor: pointer;
            padding: 8px 12px;
            user-select: none;
            z-index: 102;
            background: rgba(0, 0, 0, 0.5);
            border-radius: 5px;
            transition: background 0.3s, box-shadow 0.3s;
            color: white;
        }
        #infoToggle:hover {
            background: rgba(0, 0, 0, 0.7);
            box-shadow: 0 2px 8px rgba(0,0,0,0.3);
        }
        #title {
            font-weight: bold;
            color: white;
            text-shadow: 0 2px 4px rgba(0,0,0,0.5);
            flex: 1;
            text-align: center;
        }
        #controls {
            position: absolute;
            top: 50px;
            left: 10px;
            width: 220px;
            background: rgba(0, 0, 0, 0.8);
            backdrop-filter: blur(10px);
            padding: 0;
            box-shadow: 0 4px 20px rgba(0,0,0,0.5);
            border-radius: 8px;
            opacity: 0;
            transform: translateY(-10px);
            pointer-events: none;
            transition: opacity 0.3s, transform 0.3s;
            z-index: 101;
            display: flex;
            flex-direction: column;
        }
        #controls.show {
            opacity: 1;
            transform: translateY(0);
            pointer-events: auto;
        }
        #renderer {
            width: 100%;
            height: 100%;
            position: absolute;
            top: 0;
            left: 0;
            cursor: grab;
        }
        #renderer:active {
            cursor: grabbing;
        }
        #controls button {
            background: transparent;
            color: white;
            border: none;
            padding: 12px 16px;
            border-radius: 0;
            cursor: pointer;
            font-size: 14px;
            font-weight: normal;
            transition: background 0.2s;
            text-align: left;
            width: 100%;
        }
        #controls button:first-child {
            border-radius: 8px 8px 0 0;
        }
        #controls button:hover {
            background: rgba(255, 255, 255, 0.1);
        }
        #controls button:active {
            background: rgba(255, 255, 255, 0.15);
        }
        #fileInput {
            display: none;
        }
        #controls button:not(:last-of-type) {
            border-bottom: 1px solid rgba(255, 255, 255, 0.1);
        }
        .status {
            padding: 10px 16px;
            border-radius: 0 0 8px 8px;
            font-size: 12px;
            text-align: center;
            border-top: 1px solid rgba(255, 255, 255, 0.1);
        }
        .status.connected {
            background: rgba(76, 175, 80, 0.3);
            color: #a5d6a7;
        }
        .status.disconnected {
            background: rgba(244, 67, 54, 0.3);
            color: #ef9a9a;
        }
        #info {
            background: rgba(0, 0, 0, 0.7);
            backdrop-filter: blur(10px);
            padding: 12px;
            position: absolute;
            top: 10px;
            right: 10px;
            border-radius: 5px;
            font-size: 12px;
            font-family: monospace;
            max-width: 250px;
            box-shadow: 0 4px 20px rgba(0,0,0,0.5);
            color: white;
            transition: opacity 0.3s, transform 0.3s;
        }
        #info.hidden {
            opacity: 0;
            transform: translateX(30px) scale(0.95);
            pointer-events: none;
        }
        #info div {
            margin: 3px 0;
        }
        #info strong {
            color: #8b9cff;
        }
        label {
            font-weight: bold;
            color: white;
        }
    </style>
</head>
<body>
    <div id="container">
        <div id="topBar">
            <div id="hamburger" onclick="toggleMenu()">
                <span></span>
                <span></span>
                <span></span>
            </div>
            <div id="title">3D Viewer</div>
            <div id="infoToggle" onclick="toggleInfo()">ℹ️</div>
        </div>
        <div id="controls">
            <button onclick="document.getElementById('fileInput').click()">Load Model Files</button>
            <input type="file" id="fileInput" accept=".obj,.mtl,.jpg,.jpeg,.png,.bmp,.gif" multiple onchange="loadModelFiles(event)">
            <button onclick="resetOrientation()">Reset Orientation</button>
            <button onclick="resetZoom()">Reset Zoom</button>
            <button onclick="resetCamera()">Reset Camera</button>
            <div id="status" class="status disconnected">Disconnected</div>
        </div>
        <div id="renderer">
            <div id="info" class="hidden">
                <div><strong>Quaternion Data:</strong></div>
                <div id="quatInfo">Waiting for data...</div>
                <div style="margin-top: 10px;"><strong>Model:</strong></div>
                <div id="modelInfo">No model loaded</div>
                <div style="margin-top: 10px;"><strong>Zoom:</strong></div>
                <div id="zoomInfo">Distance: 5.0</div>
                <div style="margin-top: 10px;"><strong>Controls:</strong></div>
                <div style="font-size: 10px; color: #666;">
                    <div>• Mouse wheel: Zoom</div>
                    <div>• Click + drag: Rotate</div>
                    <div>• Shift + drag: Move camera</div>
                </div>
            </div>
        </div>
    </div>

    <script src="https://cdnjs.cloudflare.com/ajax/libs/three.js/r128/three.min.js"></script>
    <script src="https://cdn.jsdelivr.net/npm/three@0.128.0/examples/js/loaders/OBJLoader.js"></script>
    <script src="https://cdn.jsdelivr.net/npm/three@0.128.0/examples/js/loaders/MTLLoader.js"></script>

    <script>
        let scene, camera, renderer, mesh;
        let currentQuat = new THREE.Quaternion(0, 0, 0, 1);
        let manualRotation = new THREE.Quaternion(0, 0, 0, 1);
        let ws;
        let defaultPosition = new THREE.Vector3();
        let modelLoaded = false;
        
        // Mouse rotation variables
        let isMouseDown = false;
        let previousMousePosition = { x: 0, y: 0 };
        let rotationSpeed = 0.005;
        
        // Zoom variables
        let baseCameraDistance = 5; // Base distance to object
        let zoomFactor = 1.0; // Multiplier for zoom (1.0 = no zoom)
        
        // Store loaded files
        let loadedObjFile = null;
        let loadedMtlFile = null;
        let loadedTextureFiles = [];

        // Initialize Three.js scene
        function init() {
            const container = document.getElementById('renderer');
            
            // Scene
            scene = new THREE.Scene();
            scene.background = new THREE.Color(0x2a2a2a);
            
            // Camera
            camera = new THREE.PerspectiveCamera(
                75,
                container.clientWidth / container.clientHeight,
                0.1,
                1000
            );
            camera.position.z = 5;
            
            // Renderer
            renderer = new THREE.WebGLRenderer({ antialias: true });
            renderer.setSize(container.clientWidth, container.clientHeight);
            container.appendChild(renderer.domElement);
            
            // Lights
            const ambientLight = new THREE.AmbientLight(0xffffff, 0.5);
            scene.add(ambientLight);
            
            const directionalLight = new THREE.DirectionalLight(0xffffff, 0.8);
            directionalLight.position.set(1, 1, 1);
            scene.add(directionalLight);
            
            const directionalLight2 = new THREE.DirectionalLight(0xffffff, 0.4);
            directionalLight2.position.set(-1, -1, -1);
            scene.add(directionalLight2);
            
            // Default cube if no model loaded
            createDefaultCube();
            
            // Handle window resize
            window.addEventListener('resize', onWindowResize);
            
            // Handle mouse wheel for zooming
            container.addEventListener('wheel', onMouseWheel, { passive: false });
            
            // Handle mouse rotation and panning
            container.addEventListener('mousedown', onMouseDown);
            container.addEventListener('mousemove', onMouseMove);
            container.addEventListener('mouseup', onMouseUp);
            container.addEventListener('mouseleave', onMouseUp);
            
            // Handle Shift key for pan mode cursor
            window.addEventListener('keydown', onKeyDown);
            window.addEventListener('keyup', onKeyUp);
            
            // Start animation loop
            animate();
            
            // Connect WebSocket
            connectWebSocket();
        }

        function toggleMenu() {
            const controls = document.getElementById('controls');
            controls.classList.toggle('show');
        }

        function toggleInfo() {
            const info = document.getElementById('info');
            info.classList.toggle('hidden');
        }

        function createDefaultCube() {
            const geometry = new THREE.BoxGeometry(2, 2, 2);
            const material = new THREE.MeshPhongMaterial({ 
                color: 0x00ff00,
                flatShading: true
            });
            mesh = new THREE.Mesh(geometry, material);
            
            // Add edges for better visibility
            const edges = new THREE.EdgesGeometry(geometry);
            const line = new THREE.LineSegments(edges, new THREE.LineBasicMaterial({ color: 0x000000 }));
            mesh.add(line);
            
            scene.add(mesh);
            defaultPosition.copy(mesh.position);
            modelLoaded = false;
            updateModelInfo('Default cube');
            
            // Point camera at the model
            camera.lookAt(mesh.position);
        }

        function onWindowResize() {
            const container = document.getElementById('renderer');
            camera.aspect = container.clientWidth / container.clientHeight;
            camera.updateProjectionMatrix();
            renderer.setSize(container.clientWidth, container.clientHeight);
        }

        function onMouseWheel(event) {
            event.preventDefault();
            
            // Zoom speed (percentage change per scroll)
            const zoomSpeed = 0.05;
            
            // Determine zoom direction
            const delta = event.deltaY > 0 ? 1 : -1;
            
            // Update zoom factor (smaller = closer, larger = farther)
            zoomFactor *= (1 + delta * zoomSpeed);
            
            // Clamp zoom factor (0.1 to 10x)
            zoomFactor = Math.max(0.1, Math.min(zoomFactor, 10));
            
            // Apply zoom to camera position
            camera.position.z = baseCameraDistance * zoomFactor;
            
            console.log('Zoom:', (1/zoomFactor).toFixed(2) + 'x', 'Camera pos:', 
                        camera.position.x.toFixed(2), camera.position.y.toFixed(2), camera.position.z.toFixed(2));
            
            // Update zoom display
            updateZoomInfo();
        }

        function updateZoomInfo() {
            const zoomEl = document.getElementById('zoomInfo');
            zoomEl.textContent = 'Zoom: ' + (1 / zoomFactor).toFixed(2) + 'x';
        }

        function onMouseDown(event) {
            isMouseDown = true;
            previousMousePosition = {
                x: event.clientX,
                y: event.clientY
            };
        }

        function onMouseMove(event) {
            if (!isMouseDown) return;
            
            const deltaMove = {
                x: event.clientX - previousMousePosition.x,
                y: event.clientY - previousMousePosition.y
            };
            
            // Check if Shift key is held - pan camera instead of rotate
            if (event.shiftKey) {
                // Pan camera (move left/right/up/down)
                const panSpeed = 0.01;
                camera.position.x -= deltaMove.x * panSpeed;
                camera.position.y += deltaMove.y * panSpeed;
            } else {
                // Rotate object
                // Create rotation quaternions for X and Y axis rotations
                const deltaRotationQuaternion = new THREE.Quaternion()
                    .setFromEuler(new THREE.Euler(
                        deltaMove.y * rotationSpeed,
                        deltaMove.x * rotationSpeed,
                        0,
                        'XYZ'
                    ));
                
                // Apply the delta rotation to the manual rotation
                manualRotation.multiplyQuaternions(deltaRotationQuaternion, manualRotation);
                manualRotation.normalize();
            }
            
            previousMousePosition = {
                x: event.clientX,
                y: event.clientY
            };
        }

        function onMouseUp() {
            isMouseDown = false;
        }

        function onKeyDown(event) {
            if (event.key === 'Shift') {
                const container = document.getElementById('renderer');
                if (!isMouseDown) {
                    container.style.cursor = 'move';
                }
            }
        }

        function onKeyUp(event) {
            if (event.key === 'Shift') {
                const container = document.getElementById('renderer');
                if (!isMouseDown) {
                    container.style.cursor = 'grab';
                }
            }
        }

        function animate() {
            requestAnimationFrame(animate);
            
            if (mesh) {
                // Apply combined rotation: manual rotation * sensor quaternion
                const combinedQuat = new THREE.Quaternion();
                combinedQuat.multiplyQuaternions(manualRotation, currentQuat);
                mesh.quaternion.copy(combinedQuat);
            }
            
            renderer.render(scene, camera);
        }

        function connectWebSocket() {
            const protocol = window.location.protocol === 'https:' ? 'wss:' : 'ws:';
            ws = new WebSocket(protocol + '//' + window.location.host + '/ws');
            
            ws.onopen = function() {
                console.log('WebSocket connected');
                updateStatus(true);
            };
            
            ws.onmessage = function(event) {
                try {
                    const data = JSON.parse(event.data);
                    // Three.js quaternion format: (x, y, z, w) = (i, j, k, real)
                    currentQuat.set(data.i, data.j, data.k, data.real);
                    currentQuat.normalize();
                    updateQuatInfo(data);
                } catch (e) {
                    console.error('Error parsing quaternion data:', e);
                }
            };
            
            ws.onerror = function(error) {
                console.error('WebSocket error:', error);
                updateStatus(false);
            };
            
            ws.onclose = function() {
                console.log('WebSocket closed. Reconnecting...');
                updateStatus(false);
                setTimeout(connectWebSocket, 3000);
            };
        }

        function updateStatus(connected) {
            const statusEl = document.getElementById('status');
            if (connected) {
                statusEl.textContent = 'Connected';
                statusEl.className = 'status connected';
            } else {
                statusEl.textContent = 'Disconnected';
                statusEl.className = 'status disconnected';
            }
        }

        function updateQuatInfo(quat) {
            const info = document.getElementById('quatInfo');
            info.innerHTML = 
                '<div>i: ' + quat.i.toFixed(4) + '</div>' +
                '<div>j: ' + quat.j.toFixed(4) + '</div>' +
                '<div>k: ' + quat.k.toFixed(4) + '</div>' +
                '<div>real: ' + quat.real.toFixed(4) + '</div>';
        }

        function updateModelInfo(text) {
            document.getElementById('modelInfo').textContent = text;
        }

        function loadModelFiles(event) {
            const files = Array.from(event.target.files);
            if (files.length === 0) return;
            
            // Separate OBJ, MTL, and texture files
            const objFile = files.find(f => f.name.toLowerCase().endsWith('.obj'));
            const mtlFile = files.find(f => f.name.toLowerCase().endsWith('.mtl'));
            const textureFiles = files.filter(f => {
                const lower = f.name.toLowerCase();
                return lower.endsWith('.jpg') || lower.endsWith('.jpeg') || 
                       lower.endsWith('.png') || lower.endsWith('.bmp') || lower.endsWith('.gif');
            });
            
            if (!objFile) {
                alert('Please select at least one .obj file');
                return;
            }
            
            console.log('Loading files:', objFile.name, mtlFile ? mtlFile.name : '(no MTL)', 
                        textureFiles.length + ' textures');
            
            // Check file size (warn if > 50MB)
            const maxSize = 50 * 1024 * 1024; // 50MB
            if (objFile.size > maxSize) {
                const sizeMB = (objFile.size / (1024 * 1024)).toFixed(2);
                if (!confirm('This file is quite large (' + sizeMB + ' MB). Loading may take a while and could freeze the browser. Continue?')) {
                    return;
                }
            }
            
            loadedObjFile = objFile;
            loadedMtlFile = mtlFile;
            loadedTextureFiles = textureFiles;
            
            // Show loading message
            updateModelInfo('Loading ' + objFile.name + '...');
            console.log('Loading file: ' + objFile.name + ' (' + (objFile.size / 1024).toFixed(2) + ' KB)');
            
            // If we have an MTL file, load it first, then load the OBJ
            if (mtlFile) {
                loadWithMaterial(objFile, mtlFile);
            } else {
                loadOBJOnly(objFile);
            }
        }

        function loadOBJOnly(objFile) {
            const reader = new FileReader();
            
            reader.onerror = function() {
                console.error('Error reading file:', reader.error);
                alert('Error reading file: ' + reader.error.message);
                updateModelInfo('Load failed');
            };
            
            reader.onload = function(e) {
                const contents = e.target.result;
                
                console.log('File read successfully, parsing OBJ...');
                console.log('Content length: ' + contents.length + ' characters');
                
                // Remove existing mesh
                if (mesh) {
                    scene.remove(mesh);
                }
                
                // Load OBJ
                const loader = new THREE.OBJLoader();
                try {
                    updateModelInfo('Parsing ' + objFile.name + '...');
                    const object = loader.parse(contents);
                    
                    console.log('OBJ parsed successfully, processing geometry...');
                    
                    // Center and scale the object
                    const box = new THREE.Box3().setFromObject(object);
                    const center = box.getCenter(new THREE.Vector3());
                    const size = box.getSize(new THREE.Vector3());
                    
                    console.log('Original model size:', size.x.toFixed(3), size.y.toFixed(3), size.z.toFixed(3));
                    
                    const maxDim = Math.max(size.x, size.y, size.z);
                    
                    // Ensure maxDim is not zero or too small
                    if (maxDim < 0.0001) {
                        console.error('Model has invalid dimensions');
                        alert('Error: Model has invalid dimensions (too small or zero size)');
                        createDefaultCube();
                        return;
                    }
                    
                    const targetSize = 4; // Target size for largest dimension
                    const scale = targetSize / maxDim;
                    
                    console.log('Scaling factor:', scale.toFixed(3));
                    console.log('Bounding box center:', center.x.toFixed(3), center.y.toFixed(3), center.z.toFixed(3));
                    
                    // First scale, then center at origin
                    object.scale.set(scale, scale, scale);
                    
                    // Recalculate bounding box after scaling
                    const scaledBox = new THREE.Box3().setFromObject(object);
                    const scaledCenter = scaledBox.getCenter(new THREE.Vector3());
                    
                    // Move object so its center is at the origin
                    object.position.set(-scaledCenter.x, -scaledCenter.y, -scaledCenter.z);
                    
                    // Apply default material if no MTL
                    let meshCount = 0;
                    object.traverse(function(child) {
                        if (child instanceof THREE.Mesh) {
                            meshCount++;
                            if (!child.material || child.material.name === '') {
                                child.material = new THREE.MeshPhongMaterial({ 
                                    color: 0x049ef4,
                                    flatShading: false
                                });
                            }
                        }
                    });
                    
                    mesh = object;
                    scene.add(mesh);
                    defaultPosition.copy(mesh.position);
                    modelLoaded = true;
                    
                    // Adjust camera distance to fit the scaled object in viewport
                    // Closer camera for better view - 1.3x the target size
                    baseCameraDistance = 4 * 1.3; // targetSize = 4, so 4 * 1.3 = 5.2
                    zoomFactor = 1.0; // Reset zoom
                    console.log('Base camera distance set to:', baseCameraDistance);
                    camera.position.set(0, 0, baseCameraDistance);
                    
                    // Ensure camera is looking at origin (no rotation)
                    camera.rotation.set(0, 0, 0);
                    camera.lookAt(0, 0, 0);
                    
                    console.log('Mesh position:', mesh.position.x.toFixed(2), mesh.position.y.toFixed(2), mesh.position.z.toFixed(2));
                    updateZoomInfo();
                    
                    console.log('Camera positioned at distance:', camera.position.z.toFixed(2));
                    
                    updateModelInfo(objFile.name + ' (' + meshCount + ' meshes)');
                    console.log('OBJ file loaded successfully - Meshes: ' + meshCount + ', Camera distance: ' + baseCameraDistance.toFixed(2));
                } catch (error) {
                    console.error('Error loading OBJ file:', error);
                    console.error('Error stack:', error.stack);
                    alert('Error loading OBJ file: ' + error.message + '\n\nCheck console for details.');
                    updateModelInfo('Load failed');
                    createDefaultCube();
                }
            };
            
            reader.readAsText(objFile);
        }

        function loadWithMaterial(objFile, mtlFile) {
            // Load MTL file first
            const mtlReader = new FileReader();
            
            mtlReader.onerror = function() {
                console.error('Error reading MTL file:', mtlReader.error);
                alert('Error reading MTL file: ' + mtlReader.error.message);
                updateModelInfo('Load failed');
            };
            
            mtlReader.onload = function(e) {
                const mtlContents = e.target.result;
                
                console.log('MTL file read successfully, reading OBJ...');
                
                // Load OBJ file
                const objReader = new FileReader();
                
                objReader.onerror = function() {
                    console.error('Error reading OBJ file:', objReader.error);
                    alert('Error reading OBJ file: ' + objReader.error.message);
                    updateModelInfo('Load failed');
                };
                
                objReader.onload = function(e) {
                    const objContents = e.target.result;
                    
                    console.log('OBJ file read successfully, parsing with materials...');
                    console.log('OBJ content length: ' + objContents.length + ' characters');
                    
                    // Create blob URLs for texture files
                    const textureMap = {};
                    loadedTextureFiles.forEach(file => {
                        const url = URL.createObjectURL(file);
                        textureMap[file.name] = url;
                        console.log('Created blob URL for texture:', file.name);
                    });
                    
                    // Remove existing mesh
                    if (mesh) {
                        scene.remove(mesh);
                    }
                    
                    try {
                        updateModelInfo('Parsing materials...');
                        
                        // Create custom loading manager to handle texture files
                        const manager = new THREE.LoadingManager();
                        
                        // Track when all textures are loaded
                        manager.onLoad = function() {
                            console.log('All textures loaded successfully');
                            // Clean up blob URLs after all textures are loaded
                            setTimeout(() => {
                                Object.values(textureMap).forEach(url => URL.revokeObjectURL(url));
                                console.log('Blob URLs cleaned up');
                            }, 100); // Small delay to ensure textures are in GPU memory
                        };
                        
                        manager.onError = function(url) {
                            console.error('Error loading texture:', url);
                        };
                        
                        manager.setURLModifier((url) => {
                            // Extract just the filename from the URL
                            const filename = url.split('/').pop().split('\\').pop();
                            
                            // If we have a blob URL for this texture, use it
                            if (textureMap[filename]) {
                                console.log('Mapping texture:', filename, '-> blob URL');
                                return textureMap[filename];
                            }
                            
                            console.warn('Texture not found in loaded files:', filename);
                            return url; // Fall back to original URL
                        });
                        
                        // Parse MTL with custom manager
                        const mtlLoader = new THREE.MTLLoader(manager);
                        const materials = mtlLoader.parse(mtlContents, '');
                        materials.preload();
                        
                        console.log('Materials parsed, parsing OBJ...');
                        updateModelInfo('Parsing geometry...');
                        
                        // Parse OBJ with materials
                        const objLoader = new THREE.OBJLoader();
                        objLoader.setMaterials(materials);
                        const object = objLoader.parse(objContents);
                        
                        console.log('OBJ parsed successfully, processing...');
                        
                        // Center and scale the object
                        const box = new THREE.Box3().setFromObject(object);
                        const center = box.getCenter(new THREE.Vector3());
                        const size = box.getSize(new THREE.Vector3());
                        
                        console.log('Original model size:', size.x.toFixed(3), size.y.toFixed(3), size.z.toFixed(3));
                        
                        const maxDim = Math.max(size.x, size.y, size.z);
                        
                        // Ensure maxDim is not zero or too small
                        if (maxDim < 0.0001) {
                            console.error('Model has invalid dimensions');
                            alert('Error: Model has invalid dimensions (too small or zero size)');
                            createDefaultCube();
                            return;
                        }
                        
                        const targetSize = 4; // Target size for largest dimension
                        const scale = targetSize / maxDim;
                        
                        console.log('Scaling factor:', scale.toFixed(3));
                        console.log('Bounding box center:', center.x.toFixed(3), center.y.toFixed(3), center.z.toFixed(3));
                        
                        // First scale, then center at origin
                        object.scale.set(scale, scale, scale);
                        
                        // Recalculate bounding box after scaling
                        const scaledBox = new THREE.Box3().setFromObject(object);
                        const scaledCenter = scaledBox.getCenter(new THREE.Vector3());
                        
                        // Move object so its center is at the origin
                        object.position.set(-scaledCenter.x, -scaledCenter.y, -scaledCenter.z);
                        
                        let meshCount = 0;
                        object.traverse(function(child) {
                            if (child instanceof THREE.Mesh) {
                                meshCount++;
                            }
                        });
                        
                        mesh = object;
                        scene.add(mesh);
                        defaultPosition.copy(mesh.position);
                        modelLoaded = true;
                        
                        // Adjust camera distance to fit the scaled object in viewport
                        // Closer camera for better view - 1.3x the target size
                        baseCameraDistance = 4 * 1.3; // targetSize = 4, so 4 * 1.3 = 5.2
                        zoomFactor = 1.0; // Reset zoom
                        console.log('Base camera distance set to:', baseCameraDistance);
                        camera.position.set(0, 0, baseCameraDistance);
                        
                        // Ensure camera is looking at origin (no rotation)
                        camera.rotation.set(0, 0, 0);
                        camera.lookAt(0, 0, 0);
                        
                        console.log('Mesh position:', mesh.position.x.toFixed(2), mesh.position.y.toFixed(2), mesh.position.z.toFixed(2));
                        updateZoomInfo();
                        
                        console.log('Camera positioned at distance:', camera.position.z.toFixed(2));
                        
                        console.log('Camera positioned at distance:', camera.position.z.toFixed(2));
                        
                        updateModelInfo(objFile.name + ' + ' + mtlFile.name + ' (' + meshCount + ' meshes)');
                        console.log('Model loaded successfully - Meshes: ' + meshCount + ', Camera distance: ' + baseCameraDistance.toFixed(2));
                    } catch (error) {
                        console.error('Error loading model with materials:', error);
                        console.error('Error stack:', error.stack);
                        alert('Error loading model with materials: ' + error.message + '\n\nCheck console for details.');
                        updateModelInfo('Load failed');
                        // Clean up blob URLs on error
                        Object.values(textureMap).forEach(url => URL.revokeObjectURL(url));
                        createDefaultCube();
                    }
                };
                
                objReader.readAsText(objFile);
            };
            
            mtlReader.readAsText(mtlFile);
        }

        function resetOrientation() {
            currentQuat.set(0, 0, 0, 1);
            manualRotation.set(0, 0, 0, 1);
            if (mesh) {
                mesh.quaternion.set(0, 0, 0, 1);
            }
            console.log('Orientation reset');
        }

        function resetZoom() {
            zoomFactor = 1.0;
            camera.position.z = baseCameraDistance;
            updateZoomInfo();
            console.log('Zoom reset to base distance:', baseCameraDistance);
        }

        function resetCamera() {
            // Reset camera position to origin (except Z distance)
            camera.position.x = 0;
            camera.position.y = 0;
            camera.position.z = baseCameraDistance;
            
            // Reset camera rotation
            camera.rotation.set(0, 0, 0);
            camera.lookAt(0, 0, 0);
            
            // Reset zoom
            zoomFactor = 1.0;
            updateZoomInfo();
            
            console.log('Camera reset to default position');
        }

        // Initialize when page loads
        window.onload = init;
    </script>
</body>
</html>
`
