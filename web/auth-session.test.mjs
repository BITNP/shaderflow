import test from 'node:test';
import assert from 'node:assert/strict';
import {createAuthSession} from './auth-session.js';

test('logout revokes the cookie session and runs cleanup once',async()=>{const calls=[];let cleaned=0;const auth=createAuthSession({request:async(path,options)=>{calls.push([path,options]);return null;},onLoggedOut:()=>{cleaned++;}});await auth.logout();await auth.logout();assert.equal(calls.length,1);assert.equal(calls[0][0],'/api/auth/logout');assert.equal(calls[0][1].method,'POST');assert.equal(cleaned,1);});

test('401 marks the browser session logged out',async()=>{let loggedOut=0;const auth=createAuthSession({onLoggedOut:()=>{loggedOut++;}});assert.equal(auth.handleUnauthorized(),true);assert.equal(loggedOut,1);assert.equal(auth.handleUnauthorized(),false);});
