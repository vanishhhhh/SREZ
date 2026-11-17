import { readFileSync, writeFileSync } from 'node:fs';
import { execFileSync, spawnSync } from 'node:child_process';
import { dirname, resolve } from 'node:path';
import { fileURLToPath } from 'node:url';
const root=resolve(dirname(fileURLToPath(import.meta.url)),'..');
const label=execFileSync('docker',['inspect','srez-mvp-qa-db-1','--format','{{index .Config.Labels "com.docker.compose.project"}}'],{encoding:'utf8'}).trim();
if(label!=='srez-mvp-qa')throw Error('QA label mismatch');
const values=Object.fromEntries(readFileSync(resolve(root,'.env'),'utf8').split(/\r?\n/).flatMap(line=>{const m=line.match(/^([A-Z0-9_]+)=(.*)$/);return m?[[m[1],m[2].trim().replace(/^(['"])(.*)\1$/,'$2')]]:[];}));
const env={...process.env,SREZ_QA_MEDIA_COPY:'1',TEST_DATABASE_URL:`postgres://srez:${encodeURIComponent(values.POSTGRES_PASSWORD)}@127.0.0.1:15433/srez?sslmode=disable`,SREZ_S3_ACCESS_KEY:values.SREZ_S3_ACCESS_KEY,SREZ_S3_SECRET_KEY:values.SREZ_S3_SECRET_KEY};
const result=spawnSync('go',['run','./cmd/qa-media-copy'],{cwd:root,env,encoding:'utf8'});
if(result.stdout.trim()){const proof=JSON.parse(result.stdout.trim());writeFileSync(resolve(root,'../backups/2026-11-17-before-mvp/QA_MEDIA_COPY_VERIFIED.json'),JSON.stringify(proof,null,2));console.log(JSON.stringify(proof));}
if(result.status!==0)throw Error('QA media copy did not finish. Source store was only read; inspect missing-object counts.');
