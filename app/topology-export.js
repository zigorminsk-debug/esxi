/* Raster diagram exports: animated GIF and multi-page PDF. All rendering stays offline. */
(() => {
'use strict';
const T=window.ESXiTopology;
const {nodeWidth:NW,nodeHeight:NH}=T.WORLD;
const FORWARD=[64,221,201], RETURN=[196,150,255];
const nextFrame=()=>new Promise(resolve=>setTimeout(resolve,0));
const canvas=(w,h)=>{const c=document.createElement('canvas');c.width=w;c.height=h;return c;};
const font=(size,weight=400)=>`${weight} ${size}px Atlas, "Segoe UI", Arial, sans-serif`;
function roundRect(c,x,y,w,h,r){
  r=Math.min(r,w/2,h/2);c.beginPath();c.moveTo(x+r,y);c.arcTo(x+w,y,x+w,y+h,r);c.arcTo(x+w,y+h,x,y+h,r);c.arcTo(x,y+h,x,y,r);c.arcTo(x,y,x+w,y,r);c.closePath();
}
function ellipsis(c,text,max){
  let s=String(text||'');if(c.measureText(s).width<=max)return s;
  while(s.length && c.measureText(s+'…').width>max)s=s.slice(0,-1);
  return s+'…';
}
function info(d){
  const r=d.resources;
  if(d.type==='host')return `${r.cpu} CPU × ${r.cores} яд. · ${r.ram} GiB`;
  if(d.type==='vm'||d.type==='vrouter')return `${r.vcpu} vCPU · ${r.ram} GiB RAM`;
  return d.model || T.TYPES[d.type].short;
}
function nicLabel(graph,end){const d=graph.devices.find(x=>x.id===end.device),n=d?.nics.find(x=>x.id===end.nic);return n ? n.name+(n.address?' · '+n.address:'') : '';}
function diagram(graph,{width=1200,maxHeight=900}={}){
  const b=T.bounds(graph,75), scale=Math.min((width-80)/b.width,(maxHeight-145)/b.height);
  const height=Math.max(330,Math.ceil(b.height*scale+145));
  const mapX=(width-b.width*scale)/2-b.x*scale, mapY=98-b.y*scale;
  const c=canvas(width,height), g=c.getContext('2d',{willReadFrequently:true});
  g.fillStyle='#0e1828';g.fillRect(0,0,width,height);
  g.fillStyle='#eff5ff';g.font=font(25,700);g.fillText(ellipsis(g,graph.title,width-75),38,41);
  g.fillStyle='#9aabc3';g.font=font(13,500);g.fillText('УЧЕБНАЯ СХЕМА  /  ДВИЖЕНИЕ УСЛОВНОЕ, НЕ РЕАЛЬНЫЙ ТРАФИК',38,67);
  g.save();g.translate(mapX,mapY);g.scale(scale,scale);
  g.fillStyle='#25364a';
  const grid=32, startX=Math.floor(b.x/grid)*grid, startY=Math.floor(b.y/grid)*grid;
  for(let x=startX;x<b.x+b.width;x+=grid)for(let y=startY;y<b.y+b.height;y+=grid){g.beginPath();g.arc(x,y,1.3,0,Math.PI*2);g.fill();}
  const endpoints=[];
  for(const l of graph.links){
    const a=graph.devices.find(d=>d.id===l.a.device),z=graph.devices.find(d=>d.id===l.b.device);if(!a||!z)continue;
    const p=T.geometry(a,z);endpoints.push({...p,active:l.active});
    g.beginPath();g.moveTo(p.x1,p.y1);g.lineTo(p.x2,p.y2);g.lineWidth=3;g.strokeStyle=l.active?'#6d809d':'#485362';g.setLineDash(l.active?[]:[8,7]);g.stroke();g.setLineDash([]);
    const drawLabel=(text,t,side)=>{
      if(!text)return;const x=p.x1+(p.x2-p.x1)*t,y=p.y1+(p.y2-p.y1)*t+side;
      g.font=font(10,600);g.textAlign=t<.5?'left':'right';g.lineWidth=4;g.lineJoin='round';g.strokeStyle='#0e1828';g.strokeText(ellipsis(g,text,145),x,y);g.fillStyle=l.active?'#b1c2d7':'#7b8796';g.fillText(ellipsis(g,text,145),x,y);
    };
    drawLabel(nicLabel(graph,l.a),.15,-8);drawLabel(nicLabel(graph,l.b),.85,16);g.textAlign='left';
  }
  for(const d of graph.devices){
    const type=T.TYPES[d.type],host=graph.devices.find(h=>h.id===d.hostId);
    roundRect(g,d.x,d.y,NW,NH,10);g.fillStyle='#17283d';g.fill();g.strokeStyle=type.color;g.lineWidth=1.5;g.stroke();
    roundRect(g,d.x+12,d.y+13,36,36,8);g.fillStyle='#263b54';g.fill();
    g.fillStyle=type.color;g.font=font(11,700);g.textAlign='center';g.fillText(({host:'H',vm:'VM',vrouter:'VR',router:'R',provider:'ISP',switch:'SW'})[d.type],d.x+30,d.y+35);g.textAlign='left';
    g.fillStyle='#94a9c4';g.font=font(10,600);g.fillText(ellipsis(g,type.short.toUpperCase(),NW-68),d.x+58,d.y+23);
    g.fillStyle='#f2f7ff';g.font=font(15,700);g.fillText(ellipsis(g,d.name,NW-70),d.x+58,d.y+44);
    g.fillStyle='#c2d0e2';g.font=font(11,500);g.fillText(ellipsis(g,info(d),NW-26),d.x+13,d.y+72);
    g.fillStyle=host?'#9fd8c9':'#8497af';g.font=font(10,500);g.fillText(ellipsis(g,host?'На '+host.name:`${d.nics.length} интерф. · ${d.model}`,NW-26),d.x+13,d.y+92);
    const addressed=d.nics.find(n=>n.address);g.fillStyle='#9db4ce';g.font=font(9,500);g.fillText(ellipsis(g,addressed?`${addressed.name} · ${addressed.address}`:`${d.nics.length} сетевых интерфейсов`,NW-26),d.x+13,d.y+108);
  }
  if(!graph.devices.length){g.fillStyle='#99aec9';g.font=font(22,600);g.fillText('Пустая схема — добавьте устройства.',b.x+50,b.y+100);}
  g.restore();
  g.fillStyle='#40ddc9';g.beginPath();g.arc(40,height-25,5,0,Math.PI*2);g.fill();
  g.fillStyle='#c496ff';g.beginPath();g.arc(225,height-25,5,0,Math.PI*2);g.fill();
  g.fillStyle='#aebed2';g.font=font(12,500);g.fillText('туда →',53,height-20);g.fillText('обратно ←',238,height-20);
  return {canvas:c,points:endpoints,scale,mapX,mapY};
}
function indexedCircle(buffer,width,height,x,y,r,color){
  const cx=Math.round(x),cy=Math.round(y),radius=Math.round(r);
  for(let dy=-radius;dy<=radius;dy++)for(let dx=-radius;dx<=radius;dx++){
    const px=cx+dx,py=cy+dy;
    if(dx*dx+dy*dy<=radius*radius && px>=0 && py>=0 && px<width && py<height)buffer[py*width+px]=color;
  }
}
async function gif(graph,progress=()=>{}){
  if(!graph.links.some(l=>l.active))throw new Error('Добавьте хотя бы одну включённую связь, чтобы сохранить движение.');
  if(!window.AtlasGIF)throw new Error('Не загружен локальный кодировщик GIF.');
  await document.fonts?.ready;
  progress(0,24);
  const {canvas:c,points,scale,mapX,mapY}=diagram(graph,{width:1200,maxHeight:900});
  const g=c.getContext('2d',{willReadFrequently:true}),pixels=g.getImageData(0,0,c.width,c.height).data;
  const {quantize,applyPalette,GIFEncoder}=window.AtlasGIF;
  // Reserve exact colors for the two directions. Quantize the static background only once.
  const palette=quantize(pixels,254,{format:'rgb565'});palette.push(FORWARD,RETURN);
  const forward=palette.length-2,back=palette.length-1,base=applyPalette(pixels,palette,'rgb565'),encoder=GIFEncoder();
  for(let i=0;i<24;i++){
    const frame=base.slice(),phase=(i/24+.06)%1;
    for(const p of points){
      if(!p.active)continue;
      const dx=p.x2-p.x1,dy=p.y2-p.y1,len=Math.hypot(dx,dy)||1,nx=-dy/len*4,ny=dx/len*4;
      const x1=mapX+(p.x1+dx*phase+nx)*scale,y1=mapY+(p.y1+dy*phase+ny)*scale;
      const x2=mapX+(p.x1+dx*(1-phase)-nx)*scale,y2=mapY+(p.y1+dy*(1-phase)-ny)*scale;
      indexedCircle(frame,c.width,c.height,x1,y1,5,forward);indexedCircle(frame,c.width,c.height,x2,y2,5,back);
    }
    encoder.writeFrame(frame,c.width,c.height,{palette:i===0?palette:undefined,delay:100,repeat:0,dispose:1});
    progress(i+1,24);await nextFrame();
  }
  encoder.finish();return new Blob([encoder.bytes()],{type:'image/gif'});
}
function resourceText(d){
  const r=d.resources;
  if(d.type==='host')return `CPU: ${r.cpu} × ${r.cores} ядер, ${r.ghz} GHz; RAM: ${r.ram} GiB; SSD: ${r.ssdCount} × ${r.ssdSize} GB; HDD: ${r.hddCount} × ${r.hddSize} GB.`;
  if(d.type==='vm'||d.type==='vrouter')return `vCPU: ${r.vcpu}, ядер на виртуальный сокет: ${r.coresPerSocket}; RAM: ${r.ram} GiB; SSD: ${r.ssd} GB; HDD: ${r.hdd} GB.`;
  return '';
}
function detailLines(graph){
  const out=[];
  for(const d of graph.devices){
    const h=graph.devices.find(x=>x.id===d.hostId);
    out.push({kind:'heading',text:`${d.name}  ·  ${T.TYPES[d.type].label}`});
    out.push({kind:'body',text:`Модель / ОС: ${d.model || 'не указана'}${h?'  |  Физический сервер: '+h.name:((d.type==='vm'||d.type==='vrouter')?'  |  Физический сервер не выбран':'')}`});
    const r=resourceText(d);if(r)out.push({kind:'body',text:r});
    if(d.notes)out.push({kind:'body',text:'Примечание: '+d.notes});
    if(!d.nics.length)out.push({kind:'body',text:'Сетевые интерфейсы отсутствуют.'});
    for(const n of d.nics){
      out.push({kind:'body',text:`• ${n.name}: IP ${n.address||'—'}; шлюз ${n.gateway||'—'}; MAC ${n.mac||'—'}; VLAN ${n.vlan||'—'}; port group ${n.portGroup||'—'}.`});
    }
    out.push({kind:'gap',text:''});
  }
  out.push({kind:'heading',text:'Линии связи'});
  if(!graph.links.length)out.push({kind:'body',text:'Нет линий связи.'});
  for(const l of graph.links){
    const a=graph.devices.find(x=>x.id===l.a.device),b=graph.devices.find(x=>x.id===l.b.device);
    out.push({kind:'body',text:`• ${a?.name} / ${nicLabel(graph,l.a)} ↔ ${b?.name} / ${nicLabel(graph,l.b)} · ${l.name||'без названия'} · ${l.speed} Gbit/s · ${l.active?'включена':'выключена'}.`});
  }
  const warnings=T.warnings(graph);if(warnings.length){out.push({kind:'heading',text:'Предупреждения по ресурсам'});for(const w of warnings)out.push({kind:'body',text:'• '+w});}
  return out;
}
function wrap(g,text,maxWidth){
  const words=String(text).split(/\s+/),lines=[];let line='';
  for(const word of words){
    if(!word)continue;
    let piece=word;
    while(piece && g.measureText(piece).width>maxWidth){
      if(line){lines.push(line);line='';}
      let n=1;while(n<piece.length && g.measureText(piece.slice(0,n+1)).width<=maxWidth)n++;
      lines.push(piece.slice(0,n));piece=piece.slice(n);
    }
    if(!piece)continue;
    const candidate=line?line+' '+piece:piece;
    if(g.measureText(candidate).width>maxWidth){if(line)lines.push(line);line=piece;}else line=candidate;
  }
  if(line)lines.push(line);return lines.length?lines:[''];
}
const PAGE_W=1754,PAGE_H=1240;
function pageBase(title,number){
  const c=canvas(PAGE_W,PAGE_H),g=c.getContext('2d');g.fillStyle='#fff';g.fillRect(0,0,PAGE_W,PAGE_H);
  g.fillStyle='#1a2b41';g.font=font(31,700);g.fillText(title,80,67);
  g.strokeStyle='#dbe4ed';g.lineWidth=2;g.beginPath();g.moveTo(80,88);g.lineTo(PAGE_W-80,88);g.stroke();
  g.fillStyle='#687a8f';g.font=font(17,400);g.fillText('ESXi Atlas · учебная схема; трафик не измеряется и не проверяется',80,PAGE_H-42);
  g.textAlign='right';g.fillText(String(number),PAGE_W-80,PAGE_H-42);g.textAlign='left';
  return c;
}
function jpegBytes(c){
  const data=c.toDataURL('image/jpeg',.86).split(',')[1],raw=atob(data),bytes=new Uint8Array(raw.length);
  for(let i=0;i<raw.length;i++)bytes[i]=raw.charCodeAt(i);return bytes;
}
function buildPDF(images){
  const encoder=new TextEncoder(),parts=[],offsets=[0];let length=0;
  const add=value=>{const bytes=typeof value==='string'?encoder.encode(value):value;parts.push(bytes);length+=bytes.length;};
  const object=(id,body)=>{offsets[id]=length;add(`${id} 0 obj\n`);body();add('\nendobj\n');};
  add('%PDF-1.4\n%ESXiAtlas\n');
  object(1,()=>add('<< /Type /Catalog /Pages 2 0 R >>'));
  object(2,()=>add(`<< /Type /Pages /Count ${images.length} /Kids [${images.map((_,i)=>(3+i*3)+' 0 R').join(' ')}] >>`));
  images.forEach((img,i)=>{
    const id=3+i*3, imageId=id+1, contentId=id+2;
    object(id,()=>add(`<< /Type /Page /Parent 2 0 R /MediaBox [0 0 841.89 595.28] /Resources << /XObject << /Im0 ${imageId} 0 R >> /ProcSet [/PDF /ImageC] >> /Contents ${contentId} 0 R >>`));
    object(imageId,()=>{add(`<< /Type /XObject /Subtype /Image /Width ${PAGE_W} /Height ${PAGE_H} /ColorSpace /DeviceRGB /BitsPerComponent 8 /Filter /DCTDecode /Length ${img.length} >>\nstream\n`);add(img);add('\nendstream');});
    const stream='q\n841.89 0 0 595.28 0 0 cm\n/Im0 Do\nQ\n';
    object(contentId,()=>{add(`<< /Length ${encoder.encode(stream).length} >>\nstream\n`);add(stream);add('endstream');});
  });
  const xref=length;add(`xref\n0 ${offsets.length}\n0000000000 65535 f \n`);
  for(let i=1;i<offsets.length;i++)add(String(offsets[i]).padStart(10,'0')+' 00000 n \n');
  add(`trailer\n<< /Size ${offsets.length} /Root 1 0 R >>\nstartxref\n${xref}\n%%EOF\n`);
  return new Blob(parts,{type:'application/pdf'});
}
async function pdf(graph,progress=()=>{}){
  await document.fonts?.ready;progress('Подготавливаем схему…');
  const images=[],cover=pageBase('Схема: '+graph.title,1),g=cover.getContext('2d');
  const d=diagram(graph,{width:1600,maxHeight:1040}).canvas;
  const scale=Math.min((PAGE_W-160)/d.width,(PAGE_H-180)/d.height);
  g.drawImage(d,(PAGE_W-d.width*scale)/2,110,d.width*scale,d.height*scale);
  images.push(jpegBytes(cover));await nextFrame();
  let page=pageBase('Устройства и линии связи',2),ctx=page.getContext('2d'),y=130,pageNumber=2;
  function flush(){images.push(jpegBytes(page));pageNumber++;page=pageBase('Устройства и линии связи · продолжение',pageNumber);ctx=page.getContext('2d');y=130;}
  for(const entry of detailLines(graph)){
    if(entry.kind==='gap'){y+=13;continue;}
    const heading=entry.kind==='heading',size=heading?24:20,lineHeight=heading?37:29;
    ctx.font=font(size,heading?700:400);
    const wrapped=wrap(ctx,entry.text,PAGE_W-180);
    for(const text of wrapped){
      if(y+lineHeight>PAGE_H-90){flush();ctx.font=font(size,heading?700:400);}
      ctx.fillStyle=heading?'#243b5b':'#46586d';ctx.fillText(text,90,y);y+=lineHeight;
    }
    if(heading)y+=5;
  }
  images.push(jpegBytes(page));progress('Собираем PDF…');await nextFrame();
  return buildPDF(images);
}
window.TopologyExport={gif,pdf,diagram};
})();
