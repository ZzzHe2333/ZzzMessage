import test from "node:test"
import assert from "node:assert/strict"
import {scopeKey,conversationSummaries,sortConversations,searchLocalMessages,previewOf,summarizeReactions} from "./chatUiState.js"
const rooms=[{id:"a",name:"Alpha"},{id:"b",name:"Beta"},{id:"lobby",name:"大厅"}]
const history=[
  {id:"1",roomId:"a",sender:"alice",text:"hello",createdAt:"2026-10-15T10:00:00Z"},
  {id:"2",roomId:"b",sender:"bob",text:"报告",createdAt:"2026-10-15T10:05:00Z"},
  {id:"3",roomId:"a",sender:"bob",text:"latest",createdAt:"2026-10-15T10:10:00Z"},
]
test("scoped keys never mix accounts or servers",()=>{
 assert.notEqual(scopeKey("https://a","user"),scopeKey("https://b","user"))
 assert.notEqual(scopeKey("https://a","alice"),scopeKey("https://a","bob"))
})
test("unread counts, previews and pin sorting",()=>{
 const summaries=conversationSummaries(history,"alice",{a:{lastRead:"2026-10-15T10:02:00Z"}})
 assert.equal(summaries.a.unread,1)
 assert.equal(summaries.b.unread,1)
 assert.equal(previewOf(summaries.a.latest,"alice"),"latest")
 assert.equal(sortConversations(rooms,summaries,{}, "",r=>r.name)[0].id,"a")
 assert.equal(sortConversations(rooms,summaries,{b:{pinned:true}}, "",r=>r.name)[0].id,"b")
 assert.equal(conversationSummaries(history,"alice",{a:{lastRead:"2026-10-15T10:11:00Z"}}).a.unread,0)
})
test("message search matches text, sender and attachments only in permitted rooms",()=>{
 const extra={id:"4",roomId:"b",sender:"alice",text:"[文件]",attachment:{name:"report.pdf"},createdAt:"2026-10-15T10:20:00Z"}
 assert.equal(searchLocalMessages([...history,extra],"REPORT",["b"]).length,1)
 assert.equal(searchLocalMessages([...history,extra],"报告",["a"]).length,0)
 assert.equal(searchLocalMessages(history,"bob",null,1).length,1)
 assert.equal(searchLocalMessages(history,"   ").length,0)
})

test("reaction events are deduplicated, toggle off and do not create chat previews",()=>{
 const a={id:"e1",type:"reaction",roomId:"a",replyTo:"m1",sender:"bob",text:"👍",createdAt:"2026-10-15T10:11:00Z"}
 const b={...a,id:"e2",sender:"alice"}
 const m=summarizeReactions([a,a,b],"a","alice")
 assert.deepEqual(m.m1,[{emoji:"👍",count:2,mine:true}])
 assert.deepEqual(summarizeReactions([a,b,{...a,id:"e3"}],"a","alice").m1,[{emoji:"👍",count:1,mine:true}])
 assert.equal(conversationSummaries([...history,a],"alice",{}).a.latest.id,"3")
 assert.equal(searchLocalMessages([a],"👍").length,0)
})
