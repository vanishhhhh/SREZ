// Read-only index/history audit. Does not stage, commit, push, print secrets or
// scan private working files as candidates for publication.
import { readFileSync } from 'node:fs';
import { execFileSync } from 'node:child_process';
import { resolve, dirname } from 'node:path';
import { fileURLToPath } from 'node:url';

const root=resolve(dirname(fileURLToPath(import.meta.url)),'..');
const git=(...args)=>execFileSync('git',['--no-optional-locks',...args],{cwd:root,maxBuffer:128*1024*1024});
const env=Object.fromEntries(readFileSync(resolve(root,'.env'),'utf8').split(/\r?\n/).flatMap(l=>{const m=l.match(/^([A-Z][A-Z0-9_]*)=(.*)$/);return m?[[m[1],m[2].trim().replace(/^(['"])(.*)\1$/,'$2')]]:[]}));
const secrets=Object.entries(env).filter(([key,value])=>/KEY|SECRET|PASSWORD/.test(key)&&value.length>=16).map(([,value])=>value);
const example=readFileSync(resolve(root,'.env.example'),'utf8').match(/^POSTGRES_PASSWORD=(.*)$/m)?.[1]?.trim();
const defaultDB=env.POSTGRES_PASSWORD===example;
if(!defaultDB&&env.POSTGRES_PASSWORD)secrets.push(env.POSTGRES_PASSWORD);
const entries=git('ls-files','--stage','-z').toString('utf8').split('\0').filter(Boolean).map(line=>{
 const m=line.match(/^\d+ ([a-f0-9]+) (\d)\t([\s\S]+)$/);
 if(!m||m[2]!=='0')throw Error('Unmerged or invalid index entry');return {sha:m[1],path:m[3]};
});
const files=entries.map(e=>e.path);
const forbidden=files.filter(f=>/(^|\/)(?:\.secrets|node_modules|dist|build|test-results|playwright-report|backups|pgdata|postgres_data)(\/|$)/.test(f)||/(^|\/)\.env($|\.(?!example$))/.test(f)||/\.(?:dump|log|sql\.gz|pem|key|p12|pfx)$/.test(f)||/^(?:mvp|recovery)-.*\.results\.json$/.test(f)||/^demo\/catalog\/(?:result-|.*verified)/.test(f)||/^frontend\/docs\/(?:previews\/|MVP_(?:BUNDLE_CHECK|RUNTIME_CHECK|GIT_STATUS)\.)/.test(f));
const findings=[];
function scan(bytes,path,scope){
 if(bytes.includes(0))return;
 const text=bytes.toString('utf8');
 if(secrets.some(s=>text.includes(s)))findings.push({scope,path,kind:'configured-server-secret'});
 if(/-----BEGIN (?:RSA |EC |OPENSSH )?PRIVATE KEY-----/.test(text))findings.push({scope,path,kind:'private-key'});
 if(/\beyJ[A-Za-z0-9_-]{10,}\.[A-Za-z0-9_-]{10,}\.[A-Za-z0-9_-]{20,}\b/.test(text))findings.push({scope,path,kind:'literal-jwt-review-required'});
 if(/(?:verify-email|reset-password)[?#][^\s'"]*token=[A-Za-z0-9_-]{43}(?![A-Za-z0-9_-])/.test(text)||/\/(?:w|wishlists\/shared)\/[A-Za-z0-9_-]{43}(?![A-Za-z0-9_-])/.test(text))findings.push({scope,path,kind:'literal-capability-review-required'});
}
function blobs(objects,scope){
 if(!objects.length)return;
 const batch=execFileSync('git',['cat-file','--batch'],{cwd:root,input:objects.map(o=>o.sha).join('\n')+'\n',maxBuffer:128*1024*1024});
 let offset=0;
 for(const object of objects){const end=batch.indexOf(10,offset);const header=batch.subarray(offset,end).toString();const m=header.match(/^[a-f0-9]+ blob (\d+)$/);if(!m)throw Error('Unexpected Git blob batch');const size=Number(m[1]);const content=batch.subarray(end+1,end+1+size);scan(content,object.path,scope);offset=end+1+size+1;if(scope==='index')indexBytes+=size;}
}
let indexBytes=0;
blobs(entries,'index');
// Inspect only source-like historical blobs; no stdout contains their contents.
const objects=git('rev-list','--objects','--all').toString('utf8').trim().split('\n').map(l=>{const i=l.indexOf(' ');return i>0?{sha:l.slice(0,i),path:l.slice(i+1)}:null;}).filter(o=>o&&/\.(?:go|ts|tsx|js|mjs|json|ya?ml|md|env|example|ps1|sh)$|(^|\/)\.env(?:\.|$)/.test(o.path));
const historicalBlobs=objects.length;
blobs(objects,'history');
const report={indexFiles:files.length,indexBytes,configuredSecretValuesChecked:secrets.length,historicalBlobsChecked:historicalBlobs,forbiddenPaths:forbidden,findings,publicExampleDatabasePasswordInUse:defaultDB,note:defaultDB?'Existing local database uses the previously public development example password. Rotate before any external use; no password or data changed by this audit.':'No match to public DB example.',readOnly:true};
console.log(JSON.stringify(report,null,2));
if(forbidden.length||findings.length)process.exitCode=1;
