// The Plan view, drawn in a browser engine.
//
// The map is a canvas, which is the same shape of work as any other toolkit:
// project, draw links, draw nodes, draw labels. Everything around it is
// ordinary layout, which is the point of the comparison.
const fs = require('fs')
const path = require('path')

const KIND_COLOUR = {
  'companion': '#5cbfa8', 'room-server': '#dd9e69', 'sdr-observer': '#9aa4b2',
  'emitter': '#e08a76', 'advanced-repeater': '#8fb3ff', 'simple-repeater': '#6ea8fe',
}
const shortKind = k => ({
  'simple-repeater': 'repeater', 'advanced-repeater': 'advanced',
  'sdr-observer': 'observer', 'room-server': 'room server',
}[k] || k)

function load(file) {
  const raw = JSON.parse(fs.readFileSync(file, 'utf8'))
  const nodes = (raw.nodes || [])
    .filter(n => n.Position && (n.Position.Lat || n.Position.Lon))
    .map(n => ({
      name: n.Name, kind: n.Kind, lat: n.Position.Lat, lon: n.Position.Lon,
      height: n.HeightAGLm, tx: n.TxPowerDBm, regions: n.Regions || [],
    }))
    .sort((a, b) => a.name.localeCompare(b.name))
  const links = []
  const hav = (a, b) => {
    const R = 6371, d = x => x * Math.PI / 180
    const dLat = d(b.lat - a.lat), dLon = d(b.lon - a.lon)
    const h = Math.sin(dLat / 2) ** 2 +
      Math.cos(d(a.lat)) * Math.cos(d(b.lat)) * Math.sin(dLon / 2) ** 2
    return 2 * R * Math.asin(Math.sqrt(h))
  }
  for (let i = 0; i < nodes.length; i++)
    for (let j = i + 1; j < nodes.length; j++)
      if (hav(nodes[i], nodes[j]) < 18) links.push([i, j])
  return { name: raw.name, nodes, links }
}

const fixture = process.env.MESHBENCH_FIXTURE ||
  path.join(__dirname, '..', '..', 'fixtures', 'fixture-fife-strict.json')
const sc = load(fixture)
let selected = 0

document.getElementById('counts').textContent =
  `${sc.nodes.length} nodes   ${sc.links.length} links   seed 9001`
document.getElementById('fixname').textContent = sc.name

const cv = document.getElementById('map')
const ctx = cv.getContext('2d')

function project(w, h, pad) {
  const lats = sc.nodes.map(n => n.lat), lons = sc.nodes.map(n => n.lon)
  const minLat = Math.min(...lats), maxLat = Math.max(...lats)
  const minLon = Math.min(...lons), maxLon = Math.max(...lons)
  const cos = Math.cos((minLat + maxLat) / 2 * Math.PI / 180)
  const spanX = (maxLon - minLon) * cos, spanY = maxLat - minLat
  const s = Math.min((w - 2 * pad) / spanX, (h - 2 * pad) / spanY)
  const offX = (w - spanX * s) / 2, offY = (h - spanY * s) / 2
  for (const n of sc.nodes) {
    n.x = offX + (n.lon - minLon) * cos * s
    n.y = offY + (maxLat - n.lat) * s
  }
}

function draw() {
  const dpr = window.devicePixelRatio || 1
  const w = cv.clientWidth, h = cv.clientHeight
  cv.width = w * dpr; cv.height = h * dpr
  ctx.setTransform(dpr, 0, 0, dpr, 0, 0)
  ctx.clearRect(0, 0, w, h)
  project(w, h, 46)

  ctx.lineWidth = 1
  ctx.strokeStyle = 'rgba(110,168,254,0.20)'
  ctx.beginPath()
  for (const [a, b] of sc.links) {
    ctx.moveTo(sc.nodes[a].x, sc.nodes[a].y)
    ctx.lineTo(sc.nodes[b].x, sc.nodes[b].y)
  }
  ctx.stroke()

  sc.nodes.forEach((n, i) => {
    if (i === selected) {
      ctx.strokeStyle = '#6ea8fe'; ctx.lineWidth = 1.6
      ctx.beginPath(); ctx.arc(n.x, n.y, 11, 0, 7); ctx.stroke()
    }
    ctx.fillStyle = KIND_COLOUR[n.kind] || '#6ea8fe'
    ctx.beginPath(); ctx.arc(n.x, n.y, i === selected ? 7 : 5, 0, 7); ctx.fill()
  })

  ctx.font = '11px -apple-system, Segoe UI, Roboto, sans-serif'
  sc.nodes.forEach((n, i) => {
    if (n.kind === 'simple-repeater' && i % 3 !== 0 && i !== selected) return
    ctx.fillStyle = i === selected ? '#e6e9ee' : '#9aa4b2'
    ctx.fillText(n.name, n.x + 10, n.y - 6)
  })
  ctx.fillStyle = '#78827f'
  ctx.fillText('20 km    Elevation: AWS terrarium    (c) OpenStreetMap', 18, h - 18)
}

function rows(filter = '') {
  const want = filter.trim().toLowerCase()
  const el = document.getElementById('rows')
  el.innerHTML = ''
  sc.nodes.forEach((n, i) => {
    if (want && !n.name.toLowerCase().includes(want) &&
        !n.kind.toLowerCase().includes(want)) return
    const d = document.createElement('div')
    d.className = 'row' + (i === selected ? ' on' : '')
    d.innerHTML = `<span class="sw" style="background:${KIND_COLOUR[n.kind]}"></span>` +
      `<span>${n.name}</span><span class="k">${shortKind(n.kind)}</span>`
    d.onclick = () => { selected = i; rows(document.getElementById('filter').value); inspector(); draw() }
    el.appendChild(d)
  })
}

function inspectorHTML() {
  const n = sc.nodes[selected]
  if (!n) return '<span style="color:var(--dim)">nothing selected</span>'
  const r = [['name', n.name], ['kind', shortKind(n.kind)],
    ['latitude', n.lat.toFixed(5)], ['longitude', n.lon.toFixed(5)],
    ['height', `${n.height} m above ground`], ['transmit power', `${n.tx} dBm`],
    ['regions', n.regions.join(', ') || 'none']]
  return '<table>' + r.map(([k, v]) => `<tr><td class="k">${k}</td><td>${v}</td></tr>`).join('') + '</table>'
}
function inspector() { document.getElementById('insp').innerHTML = inspectorHTML() }

// A real second window, which in Electron is a browser window the operating
// system treats like any other.
document.getElementById('pop').onclick = () => {
  const w = window.open('', 'inspector', 'width=420,height=620')
  w.document.title = 'Inspector - MeshBench'
  w.document.body.style.cssText =
    'margin:0;padding:14px;background:#171b20;color:#e6e9ee;font:13px -apple-system,sans-serif'
  w.document.body.innerHTML =
    '<h2 style="font:600 11px ui-monospace,monospace;letter-spacing:.14em;' +
    'text-transform:uppercase;color:#9aa4b2">Inspector</h2>' +
    inspectorHTML().replace(/var\(--dim\)/g, '#9aa4b2')
}

document.getElementById('filter').oninput = e => rows(e.target.value)
window.onresize = draw
rows(); inspector(); draw()
