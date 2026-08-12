import { useEffect, useRef, useState } from 'react'
import * as THREE from 'three'
import { GLTFLoader } from 'three/examples/jsm/loaders/GLTFLoader.js'
import { OBJLoader } from 'three/examples/jsm/loaders/OBJLoader.js'
import { OrbitControls } from 'three/examples/jsm/controls/OrbitControls.js'
import { MeshoptDecoder } from 'three/examples/jsm/libs/meshopt_decoder.module.js'
import { RoomEnvironment } from 'three/examples/jsm/environments/RoomEnvironment.js'
import modelUrl from './assets/model/SUPERSTRIKE.glb?url'
import atlasTextureUrl from './assets/model/SUPERSTRIKE_ATLAS.jpg?url'
import g502BodyUrl from './assets/devices/g502-se-hero-model/model_0.obj?url'
import g502DetailsUrl from './assets/devices/g502-se-hero-model/model_1.obj?url'
import g502BodyColorUrl from './assets/devices/g502-se-hero-model/material-001-basecolor.webp?url'
import g502BodyMetallicUrl from './assets/devices/g502-se-hero-model/material-001-metallic.webp?url'
import g502BodyNormalUrl from './assets/devices/g502-se-hero-model/material-001-normal.webp?url'
import g502BodyRoughnessUrl from './assets/devices/g502-se-hero-model/material-001-roughness.webp?url'
import g502DetailColorUrl from './assets/devices/g502-se-hero-model/material-002-basecolor.webp?url'
import g502DetailEmissiveUrl from './assets/devices/g502-se-hero-model/material-002-emissive.webp?url'
import g502DetailMetallicUrl from './assets/devices/g502-se-hero-model/material-002-metallic.webp?url'
import g502DetailNormalUrl from './assets/devices/g502-se-hero-model/material-002-normal.webp?url'
import g502DetailRoughnessUrl from './assets/devices/g502-se-hero-model/material-002-roughness.webp?url'

type ViewerState = 'loading' | 'ready' | 'error'

export default function ThreeMouseViewer({active = true, onPrepared, modelId = 'superstrike'}: {active?: boolean;onPrepared?:()=>void;modelId?:string}) {
  const host = useRef<HTMLDivElement>(null)
  const activeRef = useRef(active)
  const [state, setState] = useState<ViewerState>('loading')

  useEffect(() => { activeRef.current = active }, [active])

  useEffect(() => {
    const container = host.current
    if (!container) return

    setState('loading')

    let frame = 0
    let disposed = false
    const scene = new THREE.Scene()
    const camera = new THREE.PerspectiveCamera(31, 1, .01, 100)
    camera.position.set(3.2, 2.15, 3.7)

    let renderer: THREE.WebGLRenderer
    try {
      renderer = new THREE.WebGLRenderer({ antialias: true, alpha: true, powerPreference: 'high-performance' })
    } catch {
      setState('error')
      onPrepared?.()
      return
    }
    renderer.setPixelRatio(Math.min(window.devicePixelRatio, 2))
    renderer.outputColorSpace = THREE.SRGBColorSpace
    renderer.toneMapping = THREE.ACESFilmicToneMapping
    renderer.toneMappingExposure = .78
    const isG502 = modelId === 'g502-se-hero'
    renderer.domElement.setAttribute('aria-label', `Interactive 3D model of the ${isG502?'G502 HERO':'PRO X2 Superstrike'}`)
    container.appendChild(renderer.domElement)

    const controls = new OrbitControls(camera, renderer.domElement)
    controls.enableDamping = true
    controls.dampingFactor = .055
    controls.enablePan = false
    controls.minDistance = 3.2
    controls.maxDistance = 7.2
    controls.autoRotate = true
    controls.autoRotateSpeed = .72
    controls.target.set(0, .02, 0)

    const pmrem = new THREE.PMREMGenerator(renderer)
    pmrem.compileEquirectangularShader()
    const environmentTarget = pmrem.fromScene(new RoomEnvironment(), .045)
    const studioEnvironment = environmentTarget.texture
    pmrem.dispose()

    scene.add(new THREE.HemisphereLight(0xdfe5eb, 0x121316, .68))
    const key = new THREE.DirectionalLight(0xfff4e8, 1.65)
    key.position.set(-4.5, 5.2, 4.2); scene.add(key)
    const fill = new THREE.DirectionalLight(0xb9d8f2, .48)
    fill.position.set(4.5, 2.2, 4.8); scene.add(fill)
    const rim = new THREE.DirectionalLight(0xd9eaff, 1.05)
    rim.position.set(3.8, 4.2, -4.8); scene.add(rim)
    const top = new THREE.DirectionalLight(0xffffff, .26)
    top.position.set(-.5, 7, -.5); scene.add(top)

    const manager = new THREE.LoadingManager()
    let modelLoaded = false
    manager.onLoad = () => {
      if (!disposed && modelLoaded) { setState('ready'); onPrepared?.() }
    }
    const textureLoader = new THREE.TextureLoader(manager)
    const prepareTexture = (url:string,color=false) => {
      const texture=textureLoader.load(url)
      if(color)texture.colorSpace=THREE.SRGBColorSpace
      texture.wrapS=THREE.ClampToEdgeWrapping;texture.wrapT=THREE.ClampToEdgeWrapping
      texture.anisotropy=renderer.capabilities.getMaxAnisotropy()
      return texture
    }
    const addModel = (object:THREE.Object3D,rotation:number) => {
      const bounds = new THREE.Box3().setFromObject(object)
      const size = bounds.getSize(new THREE.Vector3())
      const center = bounds.getCenter(new THREE.Vector3())
      const scale = 2.7 / Math.max(size.x, size.y, size.z)
      object.position.copy(center.multiplyScalar(-1))
      const model = new THREE.Group()
      model.scale.setScalar(scale)
      model.rotation.y = rotation
      model.add(object)
      scene.add(model)
      modelLoaded = true
    }
    if (isG502) {
      const material001 = new THREE.MeshStandardMaterial({
        map:prepareTexture(g502BodyColorUrl,true),metalnessMap:prepareTexture(g502BodyMetallicUrl),
        normalMap:prepareTexture(g502BodyNormalUrl),roughnessMap:prepareTexture(g502BodyRoughnessUrl),
        color:0xffffff,metalness:1,roughness:1,envMap:studioEnvironment,envMapIntensity:.42,
      })
      material001.name='G502 material 001'
      const material002 = new THREE.MeshStandardMaterial({
        map:prepareTexture(g502DetailColorUrl,true),emissiveMap:prepareTexture(g502DetailEmissiveUrl,true),
        metalnessMap:prepareTexture(g502DetailMetallicUrl),normalMap:prepareTexture(g502DetailNormalUrl),
        roughnessMap:prepareTexture(g502DetailRoughnessUrl),color:0xffffff,emissive:0xffffff,
        emissiveIntensity:1.15,metalness:1,roughness:1,envMap:studioEnvironment,envMapIntensity:.46,
      })
      material002.name='G502 material 002'
      const parts = new THREE.Group(), loader = new OBJLoader(manager)
      let loadedParts=0
      const loaded=(object:THREE.Group,material:THREE.MeshStandardMaterial)=>{
        if(disposed)return
        object.traverse(child=>{if(child instanceof THREE.Mesh)child.material=material})
        parts.add(object)
        if(++loadedParts===2)addModel(parts,Math.PI*.62)
      }
      const failed=()=>{if(!disposed){setState('error');onPrepared?.()}}
      // model_0 is the mesh whose UVs cover Material 002's logo and DPI
      // emissive pixels. The source folder did not include its MTL files, so
      // this relationship has to be restored explicitly.
      loader.load(g502BodyUrl,object=>loaded(object,material002),undefined,failed)
      loader.load(g502DetailsUrl,object=>loaded(object,material001),undefined,failed)
    } else {
      const atlasTexture = prepareTexture(atlasTextureUrl,true)
      atlasTexture.flipY = true
      const loader = new GLTFLoader(manager).setMeshoptDecoder(MeshoptDecoder)
      loader.load(modelUrl, gltf => {
        if (disposed) return
        const object = gltf.scene
        object.traverse(child => {
        if (!(child instanceof THREE.Mesh)) return
        const materials = Array.isArray(child.material) ? child.material : [child.material]
        for (const material of materials) {
          const textured = material as THREE.MeshPhongMaterial & THREE.MeshStandardMaterial
          const materialName = material.name.toLowerCase()
          const atlasSlot = materialName.includes('bottompart') ? 0 : materialName.includes('click') ? 1 : materialName.includes('mainbody') ? 2 : -1
          if (atlasSlot >= 0) {
            textured.map = atlasTexture
            const uv = child.geometry.getAttribute('uv')
            for (let index = 0; index < uv.count; index++) uv.setX(index, (uv.getX(index) + atlasSlot) / 3)
            uv.needsUpdate = true
          }
          if (textured.map) {
            textured.color.set(0xeeeeee)
            textured.map.colorSpace = THREE.SRGBColorSpace
            textured.map.anisotropy = renderer.capabilities.getMaxAnisotropy()
          } else if (materialName.includes('white')) {
            textured.color.set(0xd0d3d6)
          }
          textured.envMap = studioEnvironment
          if ('envMapIntensity' in textured) textured.envMapIntensity = materialName.includes('click') ? .46 : .32
          if ('roughness' in textured) textured.roughness = Math.max(textured.roughness, materialName.includes('click') ? .42 : .52)
          if ('metalness' in textured && atlasSlot >= 0) textured.metalness = Math.min(textured.metalness, .08)
          if ('reflectivity' in textured) textured.reflectivity = materialName.includes('click') ? .14 : .08
          if ('shininess' in textured) textured.shininess = Math.min(textured.shininess || 24, 30)
          textured.needsUpdate = true
        }
        })
        addModel(object,Math.PI*.72)
      }, undefined, () => {
        if (!disposed) { setState('error'); onPrepared?.() }
      })
    }

    const resize = () => {
      const width = Math.max(1, container.clientWidth), height = Math.max(1, container.clientHeight)
      camera.aspect = width / height; camera.updateProjectionMatrix(); renderer.setSize(width, height, false)
    }
    const observer = new ResizeObserver(resize)
    observer.observe(container); resize()

    const animate = () => {
      if (activeRef.current) { controls.update(); renderer.render(scene, camera) }
      frame = requestAnimationFrame(animate)
    }
    animate()

    return () => {
      disposed = true; cancelAnimationFrame(frame); observer.disconnect(); controls.dispose()
      scene.traverse(child => {
        if (!(child instanceof THREE.Mesh)) return
        child.geometry.dispose()
        const materials = Array.isArray(child.material) ? child.material : [child.material]
        materials.forEach(material => {
          const map = (material as THREE.MeshStandardMaterial).map
          map?.dispose(); material.dispose()
        })
      })
      environmentTarget.dispose()
      renderer.dispose(); renderer.domElement.remove()
    }
  }, [modelId])

  return <div className={`three-mouse-viewer ${state}`} ref={host}>
    <div className="three-stage-copy"><span>INTERACTIVE VIEW</span><strong>Drag to rotate · Scroll to zoom</strong></div>
    {state === 'loading' && <div className="three-stage-status"><i/><span>Preparing 3D view</span></div>}
    {state === 'error' && <div className="three-stage-status error"><span>3D preview unavailable</span></div>}
  </div>
}
