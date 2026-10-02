/* Safe camera-script interpreter and camera math. Browser ES module; no eval/Function. */
const DEFAULT_FOV = 60;
const MAX_STEPS = 10000;
const MATH = Object.freeze({
  abs: Math.abs, acos: Math.acos, asin: Math.asin, atan: Math.atan, atan2: Math.atan2,
  ceil: Math.ceil, cos: Math.cos, exp: Math.exp, floor: Math.floor, log: Math.log,
  max: Math.max, min: Math.min, pow: Math.pow, round: Math.round, sign: Math.sign,
  sin: Math.sin, sqrt: Math.sqrt, tan: Math.tan, trunc: Math.trunc,
  PI: Math.PI, E: Math.E
});
const DEFAULTS = Object.freeze({pos:[0,0,0], euler:[0,0,0], fov:DEFAULT_FOV});

function tokenize(src) {
  if (typeof src !== 'string') return [];
  if (src.length > 200000) throw Error('camera script too large');
  const out=[]; let i=0;
  while (i<src.length) {
    const c=src[i]; if (/\s/.test(c)) { i++; continue; }
    if (c==='/' && src[i+1]==='/') { i+=2; while(i<src.length&&src[i]!='\n')i++; continue; }
    if (c==='/' && src[i+1]==='*') { const e=src.indexOf('*/',i+2); if(e<0)throw Error('unterminated comment'); i=e+2; continue; }
    if (/[A-Za-z_$]/.test(c)) { const s=i++; while(i<src.length&&/[A-Za-z0-9_$]/.test(src[i]))i++; out.push({t:'id',v:src.slice(s,i)}); continue; }
    if (/[0-9.]/.test(c) && (c!=='.'||/[0-9]/.test(src[i+1]||''))) { const m=src.slice(i).match(/^(?:\d+\.?\d*|\.\d+)(?:[eE][+-]?\d+)?/); if(!m)throw Error('bad number'); i+=m[0].length; out.push({t:'num',v:Number(m[0])}); continue; }
    if (c==='"'||c==="'") { const q=c,s=i++; let v=''; while(i<src.length&&src[i]!==q){ if(src[i]==='\\'){i++; if(i>=src.length)break; const z=src[i++]; v+=z==='n'?'\n':z==='r'?'\r':z==='t'?'\t':z; } else v+=src[i++]; } if(src[i++]!==q)throw Error('unterminated string'); out.push({t:'str',v}); continue; }
    const op=['===','!==','>>>','**','==','!=','<=','>=','&&','||','++','--','=>','<<','>>'].find(x=>src.startsWith(x,i));
    if(op){out.push({t:'op',v:op});i+=op.length;continue;} if('{}[]();,:?.+-*/%<>=!&|'.includes(c)){out.push({t:'op',v:c});i++;continue;} throw Error(`unexpected character ${c}`);
  } out.push({t:'eof',v:''}); return out;
}
function parse(src) {
  const ts=tokenize(src); let p=0; const peek=()=>ts[p]; const take=(v)=>{if(v&&peek().v!==v)throw Error(`expected ${v}`);return ts[p++];};
  const is=(v)=>peek().v===v; const name=()=>{const t=take();if(t.t!=='id'||['constructor','prototype','__proto__','this','window','globalThis','document','eval','Function'].includes(t.v))throw Error('invalid identifier');return t.v;}; const semi=()=>{if(is(';'))take(';');};
  function program(stop){const a=[];while(peek().t!=='eof'&&!is(stop))a.push(statement());return {k:'block',a};}
  function statement(){
    if(is(';')){take(';');return {k:'lit',v:undefined};}
    if(is('{')){take('{');const b=program('}');take('}');return b;}
    if(is('var')||is('let')||is('const')){take();const a=[];do{const n=name();let e={k:'lit',v:undefined};if(is('=')){take('=');e=expr();}a.push({k:'decl',n,e});}while(is(',')&&take(','));semi();return {k:'seq',a};}
    if(is('if')){take('if');take('(');const c=expr();take(')');const y=statement();const n=is('else')?(take('else'),statement()):null;return {k:'if',c,y,n};}
    if(is('switch')){take('switch');take('(');const x=expr();take(')');take('{');const cs=[];while(!is('}')&&peek().t!=='eof'){let test=null;if(is('case')){take('case');test=expr();take(':');}else if(is('default')){take('default');take(':');}else throw Error('expected case/default');const body=[];while(!is('case')&&!is('default')&&!is('}')&&peek().t!=='eof')body.push(statement());cs.push({test,body});}take('}');return {k:'switch',x,cs};}
    if(is('break')){take('break');semi();return {k:'break'};}
    const e=expr();semi();return {k:'expr',e};
  }
  const prec={'||':1,'&&':2,'|':3,'&':4,'==':5,'!=':5,'===':5,'!==':5,'<':6,'>':6,'<=':6,'>=':6,'<<':7,'>>':7,'+':8,'-':8,'*':9,'/':9,'%':9,'**':10};
  function expr(min=0){let l=prefix();while(true){if(is('?')&&min<=0){take('?');const y=expr();take(':');l={k:'tern',c:l,y,n:expr(0)};continue;}const op=peek().v,q=prec[op];if(q===undefined||q<min)break;take();const r=expr(op==='**'?q:q+1);l={k:'bin',op,l,r};}if(min===0&&['=','+=','-=','*=','/='].includes(peek().v)){const op=take().v;l={k:'assign',op,l,r:expr(0)};}return l;}
  function prefix(){const t=peek();if(['!','-','+','~'].includes(t.v)){take();return {k:'un',op:t.v,x:prefix()};}let n;if(t.t==='num'||t.t==='str'){take();n={k:'lit',v:t.v};}else if(t.t==='id'){take();if(t.v==='true'||t.v==='false'||t.v==='null')n={k:'lit',v:t.v==='true'?true:t.v==='false'?false:null};else {p--;n={k:'id',n:name()};}}else if(is('(')){take();n=expr();take(')');}else if(is('[')){take();const a=[];while(!is(']')){a.push(expr());if(!is(','))break;take(',');}take(']');n={k:'arr',a};}else throw Error('expression expected');
    while(true){if(is('[')){take('[');const ix=expr();take(']');n={k:'mem',o:n,i:ix};}else if(is('.')){take('.');n={k:'mem',o:n,i:{k:'lit',v:name()}};}else if(is('(')){take('(');const a=[];while(!is(')')){a.push(expr());if(!is(','))break;take(',');}take(')');n={k:'call',f:n,a};}else if(is('++')||is('--')){const op=take().v;n={k:'post',op,x:n};}else break;} return n;}
  const tree=program(); if(peek().t!=='eof')throw Error('unexpected token'); return tree;
}
function finite(v){return typeof v==='number'&&Number.isFinite(v)?v:undefined;}
function run(ast, env, limit=MAX_STEPS){let steps=0;const tick=()=>{if(++steps>limit)throw Error('script step limit exceeded');};
  const get=(n)=>Object.prototype.hasOwnProperty.call(env,n)?env[n]:undefined; const ref=(x)=>x.k==='id'?{get:()=>get(x.n),set:v=>{if(Object.prototype.hasOwnProperty.call(MATH,x.n))throw Error('read-only math name');env[x.n]=v;}}:x.k==='mem'?(()=>{const o=ev(x.o),i=ev(x.i);if(!((Array.isArray(o)&&Number.isInteger(i)&&i>=0&&i<1024)||(o===env.pos&&['x','y','z'].includes(i))||(o===env.euler&&['y','p','r'].includes(i))))throw Error('only bounded arrays and camera fields are allowed');return {get:()=>o[i],set:v=>{o[i]=v}};})():null;
  function ev(x){tick();switch(x.k){case'lit':return x.v;case'id':return get(x.n);case'assign':{const r=ref(x.l);if(!r)throw Error('invalid assignment target');const v=ev(x.r),a=r.get();const out=x.op==='='?v:x.op==='+='?a+v:x.op==='-='?a-v:x.op==='*='?a*v:a/v;r.set(out);return out;}case'arr':return x.a.map(ev);case'mem':return ref(x).get();case'un':{const v=ev(x.x);return x.op==='!'?!v:x.op==='-'?-Number(v):x.op==='+'?+v:~Number(v);}case'bin':{const a=ev(x.l);if(x.op==='&&')return a&&ev(x.r);if(x.op==='||')return a||ev(x.r);const b=ev(x.r);switch(x.op){case'+':return a+b;case'-':return a-b;case'*':return a*b;case'/':return a/b;case'%':return a%b;case'**':return a**b;case'==':case'===':return a===b;case'!=':case'!==':return a!==b;case'<':return a<b;case'>':return a>b;case'<=':return a<=b;case'>=':return a>=b;case'&':return a&b;case'|':return a|b;case'<<':return a<<b;case'>>':return a>>b;} }case'tern':return ev(x.c)?ev(x.y):ev(x.n);case'call':{if(x.f.k!=='id'||!Object.prototype.hasOwnProperty.call(MATH,x.f.n))throw Error('function not allowed');return MATH[x.f.n](...x.a.map(ev));}case'post':{const r=ref(x.x),v=Number(r.get())||0;r.set(v+(x.op==='++'?1:-1));return v;}}}
  function st(s){tick();switch(s.k){case'block':for(const x of s.a)st(x);break;case'lit':break;case'seq':for(const x of s.a)st(x);break;case'decl':env[s.n]=ev(s.e);break;case'expr':ev(s.e);break;case'break':throw {break:true};case'if':st(ev(s.c)?s.y:s.n||{k:'lit'});break;case'switch':{const v=ev(s.x);let runn=false;for(const c of s.cs){if(c.test===null||(!runn&&ev(c.test)===v))runn=true;if(runn)for(const z of c.body)try{st(z);}catch(e){if(e?.break)return;throw e;}}break;}}}
  // assignment operators are parsed as expressions by a small source rewrite node pass below
  st(ast); return env;
}
function compileCameraScript(source,{maxSteps=MAX_STEPS}={}){let ast;try{ast=parse(source||'');}catch(e){return {ok:false,error:e instanceof Error?e.message:String(e),run:()=>{throw e;}};}return {ok:true,run(scope={}){const env=Object.assign(Object.create(null),MATH,scope);run(ast,env,maxSteps);return env;}};}

export function rotationMatrix(euler=[0,0,0]){const [y,p,r]=euler.map(value=>Number(value)*Math.PI/180);const cy=Math.cos(y),sy=Math.sin(y),cp=Math.cos(p),sp=Math.sin(p),cr=Math.cos(r),sr=Math.sin(r); // intrinsic Y-P-R in degrees, column-major
  const Ry=[cy,0,-sy, 0,1,0, sy,0,cy], Rp=[1,0,0, 0,cp,sp, 0,-sp,cp], Rr=[cr,sr,0, -sr,cr,0, 0,0,1];
  const mul=(a,b)=>{const o=Array(9);for(let c=0;c<3;c++)for(let rr=0;rr<3;rr++)o[c*3+rr]=a[rr]*b[c*3]+a[3+rr]*b[c*3+1]+a[6+rr]*b[c*3+2];return o;};return mul(mul(Ry,Rp),Rr);}
export function computeCameraMetadata({time=0,width=1,height=1,pos=DEFAULTS.pos,euler=DEFAULTS.euler,fov=DEFAULT_FOV}={}){const vec=(value,keys)=>keys.map((key,i)=>finite(Number(Array.isArray(value)?value[i]:value?.[key]))??0);const p=vec(pos,['x','y','z']),e=vec(euler,['y','p','r']),f=Math.min(179,Math.max(1,finite(Number(fov))??DEFAULT_FOV));const rot=rotationMatrix(e);const inv=[rot[0],rot[3],rot[6],rot[1],rot[4],rot[7],rot[2],rot[5],rot[8]];return {time:Number(time)||0,pos:p,euler:e,fov:f,rot,invRot:inv,depth:Math.min(width,height)/(2*Math.tan(f*Math.PI/360))};}
export function createCameraController({initSource='',frameSource='',mode='script',manual={},maxSteps=MAX_STEPS}={}){let init=compileCameraScript(initSource,{maxSteps}), frame=compileCameraScript(frameSource,{maxSteps}), state={};let lastGood;
 const reset=()=>{state={};if(init.ok)try{state=init.run({});}catch{};}; reset();
 const evaluate=(time=0,opts={})=>{const base={time,pos:DEFAULTS.pos.slice(),euler:DEFAULTS.euler.slice(),fov:DEFAULT_FOV};if(mode==='manual')Object.assign(base,manual);else if(frame.ok)try{const e=frame.run(Object.assign({},state,{time,pos:base.pos.slice(),euler:base.euler.slice(),fov:base.fov}));base.pos=[e.pos?.x??e.pos?.[0]??base.pos[0],e.pos?.y??e.pos?.[1]??base.pos[1],e.pos?.z??e.pos?.[2]??base.pos[2]];base.euler=[e.euler?.y??e.euler?.[0]??base.euler[0],e.euler?.p??e.euler?.[1]??base.euler[1],e.euler?.r??e.euler?.[2]??base.euler[2]];base.fov=e.fov;for(const k of Object.keys(state))if(Object.prototype.hasOwnProperty.call(e,k))state[k]=e[k];}catch{reset();}const m=computeCameraMetadata(Object.assign(base,opts,{time}));lastGood=m;return m;};
 return {reset,evaluate,setScripts(a='',b=''){init=compileCameraScript(a,{maxSteps});frame=compileCameraScript(b,{maxSteps});reset();},setMode(m){mode=m;},get last(){return lastGood;}};
}
export {compileCameraScript, DEFAULT_FOV};
