// SPDX-License-Identifier: MIT
// The workspace exactly as in the approved mockup (ADR-16), with real data where UnderDoc has a backend. Sprint 2026-11-08.

const I = {
  page:'<path d="M6 3h8l4 4v14H6z"/><path d="M14 3v4h4M9 12h6M9 16h6"/>',
  folder:'<path d="M3 7h6l2 2h10v10H3z"/>',
  table:'<rect x="3" y="5" width="18" height="14" rx="1"/><path d="M3 10h18M3 15h18M9 5v14M15 5v14"/>',
  excel:'<path d="M6 3h8l4 4v14H6z"/><path d="M14 3v4h4M9 11l6 6M15 11l-6 6"/>',
  images:'<rect x="3" y="5" width="18" height="14" rx="1"/><circle cx="8.5" cy="10" r="1.5"/><path d="m21 16-5-5-8 8"/>',
  view:'<path d="M4 5h16l-6 7v6l-4 2v-8z"/>',
  db:'<ellipse cx="12" cy="6" rx="8" ry="3"/><path d="M4 6v12c0 1.7 3.6 3 8 3s8-1.3 8-3V6M4 12c0 1.7 3.6 3 8 3s8-1.3 8-3"/>',
  chev:'<path d="m9 6 6 6-6 6"/>', panel:'<rect x="3" y="5" width="18" height="14" rx="1"/><path d="M9 5v14"/>',
  list:'<path d="M9 7h11M9 12h11M9 17h11M4 7h1M4 12h1M4 17h1"/>', info:'<circle cx="12" cy="12" r="9"/><path d="M12 11v5M12 8h.01"/>',
  box:'<path d="M3 7h18v13H3zM3 7l3-3h12l3 3"/><path d="M12 11v5"/>', hd:'<rect x="3" y="6" width="18" height="12" rx="1"/><path d="M7 14h.01M11 14h6"/>'
};
const ic = n => `<svg class="i" viewBox="0 0 24 24" aria-hidden="true">${I[n]}</svg>`;
const TYPE = {folder:['folder','Folder'],page:['page','Page'],table:['table','Table'],excel:['excel','Excel file'],images:['images','Image collection'],view:['view','Database view'],db:['db','Database']};
const LOGO = 'data:image/svg+xml;base64,PHN2ZyB4bWxucz0iaHR0cDovL3d3dy53My5vcmcvMjAwMC9zdmciIHZpZXdCb3g9IjAgMCAxNzU4IDE3NTgiIHJvbGU9ImltZyIgYXJpYS1sYWJlbD0iVW5kZXJEb2MiPjx0aXRsZT5VbmRlckRvYzwvdGl0bGU+PHN0eWxlPnBhdGh7ZmlsbDojMDAwfUBtZWRpYSAocHJlZmVycy1jb2xvci1zY2hlbWU6ZGFyayl7cGF0aHtmaWxsOiNmZmZ9fTwvc3R5bGU+PHBhdGggZmlsbC1ydWxlPSJldmVub2RkIiBkPSJNNjg2IDQ5TDcwMSA1MFE3MTYgNTAgNzE5IDUxTDcxOSA1MVE3MjIgNTIgNzIzIDU0TDcyMyA1NFE3MjUgNTYgNzI2IDU4TDcyNiA1OFE3MjcgNjAgNzI3IDE1MEw3MjggMjk0TDczMCAyOTdMNzY2IDI5N0w3NjcgMjk4UTc2OCAyOTggNzc2IDI5OUw3NzYgMjk5UTc4NCAzMDAgNzkyIDMwMkw3OTIgMzAyUTgwMCAzMDUgODEzIDMxMkw4MTMgMzEyUTgyNiAzMjAgODQyIDMzNEw4NDIgMzM0UTg1NiAzNDcgODU5IDM1MUw4NTkgMzUxUTg2MiAzNTQgODYyIDM1OEw4NjIgMzYwTDgyNyAzOTZRNzkyIDQzMCA3ODkgNDM1TDc4OSA0MzVRNzg2IDQ0MCA3ODMgNDQ4TDc4MyA0NDhRNzgwIDQ1OCA3ODAgNDY1TDc3OSA0NzJMNzc4IDQ3NEw3NzggNDg2TDc3NyA0ODZMNzc3IDUwMkw3NzYgNTAyTDc3NiA1NDRMNzc3IDU0Nkw3NzcgNTUwUTc3NyA1NTQgNzgwIDU3Mkw3ODAgNTcyUTc4NCA1OTAgNzg4IDYwMEw3ODggNjAwUTc5MSA2MTAgNzk3IDYyM0w3OTcgNjIzUTgwMyA2MzYgODEyIDY0OEw4MTIgNjQ4UTgyMiA2NjIgODMzIDY3Mkw4MzMgNjcyUTg0NCA2ODMgODU4IDY5Mkw4NTggNjkyUTg3MiA3MDEgODg2IDcwOEw4ODYgNzA4UTkwMCA3MTQgOTE0IDcxOEw5MTQgNzE4UTkzMCA3MjMgOTQyIDcyNUw5NDIgNzI1UTk1NiA3MjcgOTU2IDcyOEw5NTYgNzI4TDk3NiA3MjlMOTc2IDczMEwxMDE4IDczMEwxMDIwIDcyOUwxMDMwIDcyOUwxMDMwIDcyOFExMDMwIDcyOCAxMDQwIDcyN0wxMDQwIDcyN1ExMDUwIDcyNiAxMDU2IDcyNEwxMDU2IDcyNFExMDYyIDcyMyAxMDYzIDcyM0wxMDY1IDcyNEwxMDY0IDcyNlExMDYyIDcyOCAxMDU2IDc0NEwxMDU2IDc0NFExMDUwIDc2MCAxMDQzIDc3NkwxMDQzIDc3NlExMDM2IDc5MiAxMDMyIDgwMkwxMDMyIDgwMlExMDI4IDgxNCAxMDI2IDgxNkwxMDI2IDgxNlExMDI1IDgxOCAxMDIzIDgyNEwxMDIzIDgyNFExMDIxIDgzMCAxMDA5IDg1OEwxMDA5IDg1OFE5OTcgODg2IDk5NiA4OTBMOTk2IDg5MFE5OTUgODkyIDk5NSA4OTZMOTk1IDg5NlE5OTUgOTAwIDk5NiA5MDJMOTk2IDkwMlE5OTcgOTA0IDk5OSA5MDdMOTk5IDkwN1ExMDAyIDkxMCAxMDQwIDkyNkwxMDQwIDkyNlExMDc4IDk0MSAxMDgwIDk0MkwxMDgwIDk0MlExMDgyIDk0NCAxMTA3IDk1NEwxMTA3IDk1NFExMTMyIDk2MyAxMTQwIDk2N0wxMTQwIDk2N1ExMTQ4IDk3MSAxMTY2IDk3OEwxMTY2IDk3OFExMTg0IDk4NCAxMTg2IDk4NEwxMTg2IDk4NFExMTkwIDk4NCAxMjc3IDk2MkwxMzMwIDk0OFExNDE4IDkyNiAxNDIxIDkyNEwxNDIxIDkyNFExNDI0IDkyMiAxNDMwIDkxNkwxNDMwIDkxNlExNDM1IDkxMCAxNDM4IDkwMEwxNDM4IDkwMFExNDQyIDg4OCAxNDQyIDg4MkwxNDQyIDg4MlExNDQzIDg3NiAxNDQ0IDg3MkwxNDQ0IDg3MlExNDQ1IDg2OCAxNDQ1IDg2MUwxNDQ1IDg2MVExNDQ1IDg1NCAxNDQwIDg0MEwxNDQwIDg0MFExNDM1IDgyNiAxNDI4IDgxMkwxNDI4IDgxMlExNDIwIDc5OCAxNDEzIDc5MEwxNDEzIDc5MFExNDA2IDc4MiAxMzgzIDc2MEwxMzgzIDc2MFExMzYwIDczOCAxMzU4IDczNEwxMzU4IDczNFExMzU2IDcyOCAxMzU2IDcwOUwxMzU2IDcwOVExMzU3IDY5MCAxMzU4IDY4OEwxMzU4IDY4OFExMzU4IDY4OCAxMzgxIDY3MUwxMzgxIDY3MVExNDA0IDY1NCAxNDA4IDY1MkwxNDA4IDY1MlExNDEwIDY1MSAxNDIyIDY1MEwxNDMyIDY1MEwxNDM0IDY0OUwxNDY2IDY0OFExNTAwIDY0OCAxNTE5IDY1NkwxNTE5IDY1NlExNTM4IDY2NCAxNTQyIDY2N0wxNTQyIDY2N1ExNTQ1IDY3MCAxNTQ2IDY3M0wxNTQ2IDY3M1ExNTQ4IDY3NiAxNTQ4IDcwMkwxNTQ4IDcwMlExNTQ4IDcyNiAxNTQ4IDcyOUwxNTQ4IDcyOVExNTQ3IDczMiAxNTM2IDc1NkwxNTM2IDc1NlExNTI0IDc4MCAxNTE5IDc5NUwxNTE5IDc5NVExNTE0IDgxMCAxNTE0IDgxMkwxNTE0IDgxMlExNTE0IDgxNCAxNTEyIDgyNEwxNTEyIDgyNFExNTA5IDgzNCAxNTA4IDgzOUwxNTA4IDgzOVExNTA4IDg0NCAxNTA3IDg0OEwxNTA3IDg0OFExNTA2IDg1MCAxNTA2IDg1M0wxNTA2IDg1M1ExNTA2IDg1NiAxNTA1IDg1OEwxNTA1IDg1OFExNTA0IDg2MiAxNTA0IDg2NEwxNTA0IDg2NFExNTA0IDg2NiAxNTAwIDg4NEwxNTAwIDg4NFExNDk3IDkwMiAxNDk0IDkyMUwxNDk0IDkyMVExNDkxIDk0MCAxNDg5IDk0NkwxNDg5IDk0NlExNDg3IDk1MiAxNDg1IDk1NkwxNDg1IDk1NlExNDgzIDk2MCAxNDc3IDk2NUwxNDc3IDk2NVExNDcwIDk3MSAxNDY0IDk3NEwxNDY0IDk3NFExNDU4IDk3NiAxNDQ1IDk3OEwxNDQ1IDk3OFExNDMyIDk4MSAxNDI2IDk4M0wxNDI2IDk4M1ExNDIwIDk4NSAxNDE4IDk4NUwxNDE4IDk4NVExNDE2IDk4NSAxMzkzIDk5MUwxMzkzIDk5MVExMzcwIDk5NyAxMzY4IDk5N0wxMzY4IDk5N1ExMzY2IDk5NyAxMzUyIDEwMDFMMTM1MiAxMDAxUTEzMzggMTAwNSAxMzM2IDEwMDVMMTMzNiAxMDA1UTEzMzQgMTAwNSAxMzE5IDEwMDlMMTMxOSAxMDA5UTEzMDQgMTAxMyAxMzAyIDEwMTNMMTMwMiAxMDEzUTEzMDAgMTAxMyAxMjgyIDEwMThMMTI4MiAxMDE4UTEyNjQgMTAyMyAxMjYyIDEwMjNMMTI2MiAxMDIzUTEyNjAgMTAyMyAxMjM4IDEwMjhMMTIzOCAxMDI4UTEyMTYgMTAzNCAxMjA5IDEwMzhMMTIwOSAxMDM4UTEyMDIgMTA0MSAxMTk3IDEwNDZMMTE5NyAxMDQ2UTExOTIgMTA1MiAxMTg1IDEwNjRMMTE4NSAxMDY0UTExNzggMTA3NiAxMTc2IDEwODJMMTE3NiAxMDgyUTExNzMgMTA4OCAxMTcyIDEwOTZMMTE3MiAxMDk2UTExNzAgMTEwMiAxMTcwIDExMTZMMTE2OSAxMTMwTDExNjggMTEzMkwxMTY5IDE0NjBMMTE1MyAxNDcyUTExMzYgMTQ4MyAxMTI0IDE0OTFMMTEyNCAxNDkxUTExMTAgMTQ5OSAxMDk5IDE1MDdMMTA5OSAxNTA3UTEwODggMTUxNSAxMDg0IDE1MTdMMTA4NCAxNTE3UTEwODAgMTUxOSAxMDUwIDE1MzlMMTA1MCAxNTM5UTEwMjIgMTU1OSAxMDAyIDE1NzFMMTAwMiAxNTcxUTk4MiAxNTgzIDk4MSAxNTg0TDk4MSAxNTg0UTk4MCAxNTg2IDkwNCAxNjM2TDc5NCAxNzA5TDc5MiAxNzA4UTc5MCAxNzA4IDc4MiAxNzAyTDc4MiAxNzAyUTc3NCAxNjk2IDczMCAxNjY1TDczMCAxNjY1UTY4NiAxNjM0IDY0NCAxNjAzTDY0NCAxNjAzUTYwMiAxNTcyIDU5NiAxNTY4TDU5NiAxNTY4UTU5MCAxNTY0IDU2OCAxNTQ4TDU2OCAxNTQ4UTU0NiAxNTMxIDUyOSAxNTIwTDUyOSAxNTIwUTUxMiAxNTA4IDUxMCAxNTA2TDUxMCAxNTA2UTUwOCAxNTA0IDQ3NCAxNDgwTDQ3NCAxNDgwUTQ0MCAxNDU2IDQyNiAxNDQ0TDQyNiAxNDQ0UTQxMCAxNDMzIDMzNyAxMzgxTDMwNSAxMzU3UTIzMiAxMzA1IDIyMSAxMjk2TDIxMCAxMjg4TDIxMSAxMjg1UTIxMiAxMjgyIDIyNiAxMjYyTDIyNiAxMjYyUTI0MCAxMjQyIDI1MSAxMjI4TDI1MSAxMjI4UTI2MiAxMjE0IDMxNCAxMTQwTDM2NiAxMDY5UTQxOCA5OTYgNDIwIDk5M0w0MjAgOTkzUTQyMSA5OTAgNDIyIDk4NEw0MjIgOTg0UTQyNCA5NzggNDI4IDk0OEw0MjggOTQ4UTQzMSA5MTggNDMyIDkxN0w0MzIgOTE2TDQzMiA5MTRRNDMyIDkxMCA0MzQgOTAzTDQzNCA5MDNRNDM1IDg5NiA0MzUgODkyTDQzNSA4ODhMNDM2IDg4OFE0MzYgODg4IDQzOCA4NzBMNDM4IDg3MFE0NDAgODUyIDQ0MiA4NDRMNDQyIDg0NFE0NDMgODM2IDQ0NSA4MThMNDQ1IDgxOFE0NDcgNzk4IDQ0OCA3OThMNDQ4IDc5OFE0NDggNzk4IDQ1MCA3ODRMNDUwIDc4NFE0NTEgNzcwIDQ1MiA3NjlMNDUyIDc2OEw0NTIgNzY2UTQ1MiA3NjIgNDU0IDc1NUw0NTQgNzU1UTQ1NSA3NDggNDU2IDc0MEw0NTYgNzMyTDQ1NiA3MzJRNDU3IDczMiA0NTkgNzE0TDQ1OSA3MTRRNDYxIDY5NiA0NjIgNjk1TDQ2MiA2OTRMNDYyIDY4OEw0NjMgNjg4TDQ2MyA2ODRRNDYzIDY4MiA0NjQgNjc0TDQ2NCA2NzRRNDY2IDY2NiA0NjggNjQ0TDQ2OCA2NDRRNDcxIDYyMiA0NzIgNjIyTDQ3MiA2MjJRNDcyIDYyMiA0NzQgNjAyTDQ3NCA2MDJRNDc3IDU4MiA0NzggNTgxTDQ3OCA1ODBMNDc4IDU3Mkw0NzkgNTcyTDQ3OSA1NjRMNDgwIDU2Mkw0ODAgNTU4UTQ4MCA1NTQgNDgxIDU1MEw0ODIgNTQ0TDQ4MCA1NDJRNDc5IDU0MCA0MjAgNDgwTDQyMCA0ODBRMzYwIDQyMiAzNTcgNDE2TDM1NyA0MTZRMzU0IDQxMiAzNTIgNDA2TDM1MiA0MDZRMzUxIDQwMiAzNTEgMzk0TDM1MSAzOTRRMzUxIDM4NiAzNTMgMzc5TDM1MyAzNzlRMzU1IDM3MiAzNTcgMzY4TDM1NyAzNjhRMzU5IDM2NCA0MjMgMzAxTDU5MyAxMzFRNjU2IDY3IDY2NiA1OUw2NjYgNTlRNjc0IDUxIDY4MCA1MEw2ODAgNTBRNjg0IDUwIDY4NSA1MEw2ODYgNDlaTTExOTAgMTEwTDEyMDAgMTEwUTEyMDggMTEwIDEyMTQgMTEyTDEyMTQgMTEyUTEyMjAgMTEzIDEyMzAgMTE4TDEyMzAgMTE4UTEyNDAgMTIzIDEyNDIgMTI0TDEyNDMgMTI2TDEyNDMgMTI4TDEyNDAgMTMxUTEyMzYgMTM0IDEyMzMgMTM2TDEyMzMgMTM2UTEyMzAgMTM3IDEyMjUgMTQwTDEyMjUgMTQwUTEyMjAgMTQzIDEyMDMgMTUyTDEyMDMgMTUyUTExODYgMTYwIDExNzEgMTY4TDExNzEgMTY4UTExNTYgMTc2IDExNTQgMTc4TDExNTQgMTc4UTExNTIgMTgwIDExNTAgMTgyTDExNTAgMTgyUTExNDkgMTg0IDExNDggMTg4TDExNDggMTkyTDExNDkgMTkyTDExNDkgMjA4TDExNTAgMjA4TDExNTAgMjI0TDExNTEgMjI2TDExNTMgMjY4TDExNTQgMjY4TDExNTcgMzI0TDExNTggMzI2TDExNTggMzMwUTExNTggMzM0IDExNTYgMzM3TDExNTQgMzQwTDEwODQgMzQyUTEwMTQgMzQzIDk3NSAzMzFMOTc1IDMzMVE5MzYgMzE5IDkzNSAzMTlMOTM1IDMxOVE5MzQgMzE5IDkzMiAzMThMOTMyIDMxOFE5MzAgMzE3IDkxMCAzMTJMOTEwIDMxMlE4OTIgMzA3IDg4OCAzMDZMODg4IDMwNlE4ODQgMzA0IDg4MyAzMDRMODgzIDMwNFE4ODIgMzA0IDg3OCAzMDJMODc4IDMwMlE4NzQgMzAxIDg3MyAzMDFMODczIDMwMVE4NzIgMzAxIDg2NiAyOTlMODY2IDI5OVE4NjIgMjk3IDg1OSAyOTRMODU2IDI5Mkw4NTYgMjkwUTg1NiAyOTAgODY2IDI2OUw4NjYgMjY5UTg3NyAyNDggODg4IDIyOUw4ODggMjI5UTg5OSAyMTAgOTA3IDE5NEw5MDcgMTk0UTkxNSAxNzggOTIxIDE2OUw5MjEgMTY5UTkyNyAxNjAgOTMzIDE1Nkw5MzMgMTU2UTkzOCAxNTEgOTQ0IDE0OEw5NDQgMTQ4UTk1MCAxNDYgOTY0IDE0NEw5NjQgMTQ0UTk3NiAxNDIgOTc3IDE0Mkw5NzcgMTQyUTk3OCAxNDEgOTkxIDE0MEw5OTEgMTQwUTEwMDQgMTM4IDEwMDggMTM3TDEwMDggMTM3UTEwMTIgMTM2IDEwNDkgMTMxTDEwNDkgMTMxUTEwODYgMTI2IDEwOTcgMTI0TDEwOTcgMTI0UTExMDggMTIyIDExMTEgMTIyTDExMTEgMTIyUTExMTQgMTIyIDExMTggMTIxTDExMTggMTIxUTExMjIgMTIwIDExMzggMTE4TDExMzggMTE4UTExNTYgMTE2IDExNTYgMTE2TDExNTYgMTE2UTExNTYgMTE1IDExNjYgMTE0TDExNjYgMTE0UTExNzYgMTEzIDExNzkgMTEyTDExNzkgMTEyUTExODIgMTExIDExODYgMTExTDExOTAgMTExTDExOTAgMTEwWk0xMTUwIDM4NkwxMjA4IDM4NkwxMjA4IDM4N0wxMjc2IDM4OEwxMjc2IDM4OUwxMjkyIDM4OUwxMjk0IDM5MEwxMzA2IDM5MEwxMzA4IDM5MUwxMzIwIDM5MUwxMzIwIDM5MkwxMzM0IDM5MkwxMzM0IDM5M0wxMzQ2IDM5M0wxMzQ4IDM5NEwxNDA2IDM5OEwxNDA4IDM5OUwxNDEyIDM5OVExNDE2IDM5OSAxNDE5IDQwMEwxNDE5IDQwMFExNDIyIDQwMiAxNDQ2IDQyNUwxNDQ2IDQyNVExNDcwIDQ0OCAxNDcyIDQ1MkwxNDcyIDQ1MlExNDc0IDQ1NiAxNDc0IDQ5OUwxNDczIDU0MkwxNDcyIDU0MkwxNDcyIDU1NVExNDcxIDU2OCAxNDcwIDU3NEwxNDcwIDU3NFExNDY4IDU4MCAxNDY2IDU4NkwxNDY2IDU4NlExNDY0IDU5MCAxNDYyIDU5NEwxNDYyIDU5NFExNDYwIDU5OCAxNDU3IDYwMUwxNDU3IDYwMVExNDU0IDYwNCAxNDUwIDYwNkwxNDUwIDYwNlExNDQ2IDYwNyAxNDQwIDYwN0wxNDQwIDYwN1ExNDM0IDYwNyAxNDI1IDYwNEwxNDI1IDYwNFExNDE2IDYwMSAxNDA4IDYwMEwxNDA4IDYwMFExNDAwIDU5OCAxMzk4IDU5N0wxMzk4IDU5N1ExMzk2IDU5NiAxMzU4IDU4NkwxMzU4IDU4NlExMzIwIDU3NyAxMzE4IDU3NkwxMzE4IDU3NlExMzE2IDU3NSAxMzA2IDU3M0wxMzA2IDU3M1ExMjk2IDU3MSAxMjg4IDU2OEwxMjg4IDU2OFExMjgwIDU2NiAxMjc4IDU2NkwxMjc4IDU2NlExMjc2IDU2NiAxMjY4IDU2NEwxMjY4IDU2NFExMjYwIDU2MSAxMjU3IDU1OEwxMjU3IDU1OFExMjU0IDU1NiAxMjUwIDU1MkwxMjUwIDU1MlExMjQ3IDU0OCAxMjQ0IDU0MUwxMjQ0IDU0MVExMjQxIDUzNCAxMjQwIDQ5OEwxMjM5IDQ2MkwxMTkwIDQ2M0wxMTkwIDQ2NEwxMTc5IDQ2NEwxMTc4IDQ5MEwxMTc3IDQ5MEwxMTc3IDUyMkwxMTc2IDUyNEwxMTc1IDU0NkwxMTc0IDU0OEwxMTc0IDU1MFExMTc0IDU1NCAxMTcyIDU2NEwxMTcyIDU2NFExMTcwIDU3NCAxMTY2IDU4NkwxMTY2IDU4NlExMTYxIDYwMCAxMTU0IDYxMUwxMTU0IDYxMVExMTQ4IDYyMiAxMTQ0IDYyN0wxMTQ0IDYyN1ExMTQxIDYzMiAxMTM0IDYzOEwxMTM0IDYzOFExMTI4IDY0NSAxMTE2IDY1M0wxMTE2IDY1M1ExMTA2IDY2MSAxMDk4IDY2NUwxMDk4IDY2NVExMDkwIDY2OSAxMDc4IDY3NEwxMDc4IDY3NFExMDY2IDY3OCAxMDU2IDY4MEwxMDU2IDY4MFExMDQ0IDY4MyAxMDM4IDY4NEwxMDMwIDY4NEwxMDMwIDY4NUw5ODIgNjg2TDk4MCA2ODVMOTY2IDY4NUw5NjQgNjg0TDk1NyA2ODRROTUwIDY4MyA5MzMgNjc4TDkzMyA2NzhROTE2IDY3NCA5MDQgNjY4TDkwNCA2NjhRODkwIDY2MiA4ODEgNjU2TDg4MSA2NTZRODcyIDY0OSA4NjIgNjQwTDg2MiA2NDBRODUzIDYzMCA4NDggNjI0TDg0OCA2MjRRODQ0IDYxOCA4MzggNjA4TDgzOCA2MDhRODMzIDU5OCA4MzAgNTg4TDgzMCA1ODhRODI2IDU3OCA4MjQgNTY3TDgyNCA1NjdRODIxIDU1NiA4MjAgNTQ4TDgyMCA1NDBMODE5IDU0MEw4MTkgNTE2TDgyMCA1MTZMODIxIDQ5M1E4MjIgNDcwIDgyMiA0NjhMODIyIDQ2OFE4MjMgNDY2IDgyNiA0NjFMODI2IDQ2MVE4MjggNDU2IDg1OSA0MjZMODU5IDQyNlE4OTAgMzk2IDg5MyAzOTRMODkzIDM5NFE4OTYgMzkzIDkwNiAzOTNMOTE0IDM5M0w5MTYgMzkyTDEwMTQgMzkwTDEwMTQgMzg5TDEwNTIgMzg5TDEwNTIgMzg4TDExNDggMzg3TDExNTAgMzg2Wk0xMDQ0IDQzOEwxMDQ0IDQzOUwxMDEyIDQzOUwxMDEwIDQ0MEw5NzggNDQwTDk3OCA0NDFMOTM0IDQ0Mkw5MzQgNDQzTDkzMCA0NDNROTI2IDQ0MyA5MjIgNDQ1TDkyMiA0NDVROTE2IDQ0NyA5MDMgNDYwTDkwMyA0NjBRODkwIDQ3MiA4OTAgNDc0TDg5MCA0NzRRODg5IDQ3NiA4ODggNDg0TDg4OCA0OTJMODg5IDQ5NEw4ODkgNTA4TDg5MCA1MTBMODkwIDUyNEw4OTEgNTI0TDg5MSA1NDBMODk0IDU0MUw4OTQgNTQwUTg5NiA1MzkgOTA2IDUzNEw5MDYgNTM0UTkxOCA1MjggOTUyIDUwOEw5NTIgNTA4UTk4NiA0ODcgMTAyOCA0NjRMMTAyOCA0NjRRMTA3MCA0NDAgMTA3MCA0MzlMMTA3MCA0MzhMMTA0NCA0MzhaTTU1MCA4MTZMNTUwIDgxN1E1NTAgODE4IDU1MSA4MjBMNTUxIDgyMFE1NTIgODIyIDU1OCA4NDRMNTU4IDg0NFE1NjMgODY2IDU2NCA4NjhMNTY0IDg2OFE1NjUgODcwIDU3NiA5MTZMNTc2IDkxNlE1ODggOTYyIDU4OSA5NjRMNTg5IDk2NFE1OTAgOTY2IDU5NCA5ODJMNTk0IDk4MlE1OTcgOTk2IDU5OCA5OThMNTk4IDk5OFE1OTkgMTAwMCA2MDAgMTAwNEw2MDAgMTAwNFE2MDAgMTAwOCA2MDEgMTAxMEw2MDEgMTAxMFE2MDIgMTAxMiA2MDggMTAzM0w2MTQgMTA1NEw2MjEgMTA1NlE2MjggMTA1NyA2MzQgMTA1N0w2NDAgMTA1N0w2NDAgMTA1OEw2NjQgMTA1OUw2NjQgMTA2MEw2NzggMTA2MEw2ODAgMTA2MUw2OTQgMTA2MUw2OTQgMTA2Mkw3MDYgMTA2Mkw3MDggMTA2M0w3MTggMTA2M0w3MjAgMTA2NEw3MzIgMTA2NEw3MzIgMTA2NUw3NDQgMTA2NUw3NDQgMTA2Nkw3NTkgMTA2N1E3NzQgMTA2OCA3ODAgMTA3MEw3ODAgMTA3MFE3ODYgMTA3MiA4MzUgMTA5Mkw4ODMgMTExMkw4ODMgMTExMVE4ODIgMTExMCA4NTIgMTA4N0w4NTIgMTA4N1E4MjAgMTA2NCA4MDcgMTA1M0w4MDcgMTA1M1E3OTQgMTA0MiA3OTAgMTA0MEw3OTAgMTA0MFE3ODYgMTAzNyA3NjYgMTAzMkw3NjYgMTAzMlE3NDggMTAyOCA3MzggMTAyNkw3MzggMTAyNlE3MjggMTAyNSA3MjYgMTAyNEw3MjYgMTAyNFE3MjQgMTAyMyA3MTYgMTAyMkw3MTYgMTAyMlE3MTAgMTAyMSA3MDcgMTAyMEw3MDcgMTAyMFE3MDQgMTAxOSA2ODAgMTAxNEw2NTQgMTAwOUw2MzkgOTgyUTYyNCA5NTQgNjE0IDkzNEw2MTQgOTM0UTYwMyA5MTIgNTk2IDkwMkw1OTYgOTAyUTU5MCA4OTAgNTgxIDg3Mkw1ODEgODcyUTU3MiA4NTQgNTYxIDgzNUw1NTAgODE2WiIvPjwvc3ZnPgo=';

/* Made-up example content */
const P = (intro, ...secs) => ({intro, secs});
const D = {
  net:{t:'folder',n:'Network',kids:['plan','tickets'],path:'operations/network/',why:'Default roles for everything below.',vis:'Group ops'},
  plan:{t:'page',n:'Network plan',w:'wide',kids:['vlans','ports'],path:'operations/network/plan.md',by:'A. Keller',ch:'today 09:41 by A. Keller',vis:'Group ops, inherited',
    uses:{ext:['vendor manual'],int:['VLAN list','On-call handbook'],data:['Site photos'],vals:['router.ip','tower.height'],db:['Inventory · Open tickets']},usedIn:['On-call handbook','Tower 50 m'],
    doc:P('The site has one router, two core switches and a radio link on the tower.',['Overview','<p>The router sits at <span class="v">192.0.2.1</span> (value <span class="v">{{router.ip}}</span>). The tower is <span class="v">50 m</span> high (value <span class="v">{{tower.height}}</span>), which sets the range of the radio link.</p><div class="note">Mermaid diagrams come with M-3. They render here, inside the same window.</div>'],['Core switches','<p>Both core switches form one stack. See <button class="l" data-act="open" data-id="ports">Switch ports</button>.</p><div class="tw"><table><tr><th>Name</th><th>Room</th><th>Uplink</th></tr><tr><td>core-a</td><td>Server room</td><td>10 Gbit</td></tr><tr><td>core-b</td><td>Workshop</td><td>10 Gbit</td></tr></table></div>'],['Radio link','<p>The radio link runs from the tower to the gatehouse. Check it after every storm.</p><p>Keep a spare antenna in the workshop, and note each swap in the handbook.</p>'],['Open tickets','<p>This table comes from the view <button class="l" data-act="open" data-id="tickets">Open tickets</button> and shows how old its data is.</p><div class="tw"><table><tr><th>#</th><th>Title</th><th>Owner</th></tr><tr><td>412</td><td>Replace fan in core-b</td><td>A. Keller</td></tr><tr><td>415</td><td>Label patch panel</td><td>J. Roth</td></tr></table></div>'],['Change log','<p>2026-10-02: new uplink on core-b.</p><p>2026-09-14: radio link firmware.</p><p>2026-08-30: tower antenna replaced.</p>'])},
  vlans:{t:'page',n:'VLAN list',w:'slim',kids:[],path:'operations/network/plan/vlans.md',by:'A. Keller',ch:'Oct 28',vis:'Group ops, inherited',uses:{ext:[],int:['Network plan'],data:[],vals:[],db:[]},usedIn:['Network plan'],
    doc:P('Every VLAN on the site.',['Office','<p>VLAN 10, for desks and printers.</p>'],['Devices','<p>VLAN 20, for cameras and sensors.</p>'])},
  ports:{t:'table',n:'Switch ports',w:'wide',kids:[],path:'not a file, lives in the database',by:'J. Roth',ch:'yesterday',vis:'Group ops, inherited',uses:{ext:[],int:[],data:[],vals:[],db:[]},usedIn:['Network plan'],
    tab:{note:'Native table: edit rows here, with history per row (M-6).',head:['Port','Device','VLAN','Status'],rows:[['1/1','core-b','trunk','up'],['1/2','camera gate','20','up'],['1/3','printer office','10','up'],['1/4','(free)','–','down'],['1/5','radio link','trunk','up']]}},
  tickets:{t:'view',n:'Open tickets',w:'wide',kids:[],path:'not a file, a saved query',by:'J. Roth',ch:'Oct 30',vis:'Group ops',uses:{ext:[],int:[],data:[],vals:[],db:['Inventory']},usedIn:['Network plan'],
    tab:{note:'From the database Inventory. Data is 4 minutes old (M-4).',head:['#','Title','Owner','Age'],rows:[['412','Replace fan in core-b','A. Keller','3 d'],['415','Label patch panel','J. Roth','1 d'],['416','Renew switch license','A. Keller','4 h']]}},
  logs:{t:'folder',n:'Site logs (11 children, stays closed)',kids:Array.from({length:11},(_,i)=>'log'+i),path:'operations/site-logs/',why:'More children than the limit below, so it does not open by itself.',vis:'Group ops'},
  hand:{t:'page',n:'On-call handbook',w:'slim',kids:['esc','cont'],path:'operations/on-call.md',by:'M. Sander',ch:'Oct 29',vis:'Everyone in the company',uses:{ext:['status page'],int:['Network plan'],data:[],vals:['router.ip'],db:[]},usedIn:[],
    doc:P('What to do when something breaks at night.',['First five minutes','<p>Open the <button class="l" data-act="open" data-id="plan">Network plan</button> next to this page. Check the router at <span class="v">192.0.2.1</span>.</p>'],['Who to call','<p>See the <button class="l" data-act="open" data-id="cont">Contacts</button> table below this page.</p>'],['After the incident','<p>Write down what happened on the same day.</p>'])},
  esc:{t:'page',n:'Escalation',w:'slim',kids:[],path:'operations/on-call/escalation.md',by:'M. Sander',ch:'Oct 12',vis:'Everyone, inherited',uses:{ext:[],int:[],data:[],vals:[],db:[]},usedIn:['On-call handbook'],doc:P('Who decides what, and when.',['Levels','<p>Level one fixes, level two decides.</p>'],['Times','<p>Fifteen minutes per level.</p>'])},
  cont:{t:'table',n:'Contacts',w:'slim',kids:[],path:'not a file, lives in the database',by:'M. Sander',ch:'Oct 20',vis:'Everyone, inherited',uses:{ext:[],int:[],data:[],vals:[],db:[]},usedIn:['On-call handbook'],tab:{note:'Native table. A changed number shows on every page that embeds it.',head:['Name','Role','Phone'],rows:[['A. Keller','Network','+00 555 0101'],['J. Roth','Site','+00 555 0102'],['M. Sander','Lead','+00 555 0103']]}},
  tower:{t:'page',n:'Tower 50 m',w:'full',kids:['dims','photos'],path:'projects/tower.md',by:'J. Roth',ch:'Oct 31',vis:'Project team',uses:{ext:[],int:['Network plan'],data:['Tower dimensions','Site photos'],vals:['tower.height'],db:[]},usedIn:['Network plan'],
    doc:P('This page asks for the full width, so the left navigation hides while its window is active.',['Status','<p>The tower stands at <span class="v">50 m</span>. Foundation and first segment are done.</p>'],['Dimensions','<p>From the named table in <button class="l" data-act="open" data-id="dims">Tower dimensions</button>.</p>'],['Photos','<p>See <button class="l" data-act="open" data-id="photos">Site photos</button>.</p>'])},
  dims:{t:'excel',n:'Tower dimensions',w:'wide',kids:[],path:'projects/tower/tower-dimensions.xlsx',by:'J. Roth (from the share log)',ch:'today 08:12 by J. Roth, over the share',vis:'Project team, inherited',uses:{ext:[],int:[],data:[],vals:[],db:[]},usedIn:['Tower 50 m'],
    tab:{note:'Excel file on the share: open it in Excel or LibreOffice, and the web shows the same file. Share path: \\\\server\\projects\\tower\\tower-dimensions.xlsx (M-2).',head:['Segment','Height (m)','Diameter (m)'],rows:[['Base','12','4.2'],['Middle','20','3.4'],['Top','18','2.6']]}},
  photos:{t:'images',n:'Site photos',w:'wide',kids:[],path:'projects/tower/photos/',by:'J. Roth',ch:'Oct 18',vis:'Project team, inherited',uses:{ext:[],int:[],data:[],vals:[],db:[]},usedIn:['Tower 50 m','Network plan']},
  inv:{t:'db',n:'Inventory',w:'wide',kids:['tickets2'],path:'not a file, a connection (Resources tab)',by:'A. Keller',ch:'Sep 02',vis:'Group ops',uses:{ext:[],int:[],data:[],vals:[],db:[]},usedIn:['Open tickets'],
    tab:{note:'A connection, placed here as well. It exists once: the Resources tab lists the same object.',head:['Table','Rows','Read only'],rows:[['devices','1,204','yes'],['tickets','318','yes'],['sites','14','yes']]}},
  tickets2:{t:'view',n:'Devices by site',w:'wide',kids:[],path:'not a file, a saved query',by:'A. Keller',ch:'Sep 03',vis:'Group ops, inherited',uses:{ext:[],int:[],data:[],vals:[],db:['Inventory']},usedIn:[],tab:{note:'A saved query below its database.',head:['Site','Devices'],rows:[['Main','842'],['Gatehouse','362']]}},
  n1:{t:'page',n:'Ideas for the workshop',w:'slim',kids:[],path:'not a file yet, private note',by:'you',ch:'today',vis:'Only you',uses:{ext:[],int:[],data:[],vals:[],db:[]},usedIn:[],doc:P('A private note, in your own collection.',['Shelves','<p>Move spare parts next to the door.</p>'],['Labels','<p>Print new ones.</p>'])},
};
for (let i=0;i<11;i++) D['log'+i]={t:'page',n:'Log week '+(30+i),w:'slim',kids:[],path:`operations/site-logs/week-${30+i}.md`,by:'J. Roth',ch:'Oct',vis:'Group ops, inherited',uses:{ext:[],int:[],data:[],vals:[],db:[]},usedIn:[],doc:P('Weekly site log.',['Work','<p>Routine checks.</p>'],['Notes','<p>Nothing unusual.</p>'])};
const C = [{id:'ops',n:'Operations',top:['net','logs','hand']},{id:'pro',n:'Projects',top:['tower','inv']},{id:'me',n:'My notes',top:['n1']}];
function bodyHtml(w,i){
  const d=D[w.d]; const hs=d.doc?d.doc.secs.map(s=>s[0]):[]; const sub=`<div class="sub">${TYPE[d.t][1]} · ${d.path}</div>${d.tags.length?`<div class="tagline">${tagChips(d.tags,'tagsearch')}</div>`:''}`;
  let h='';
  if(d.blocks) h+=blocksHtml(d,w,i);
  else if(d.doc) h+=(d.real?d.doc.intro:`<p>${d.doc.intro}</p>`)+d.doc.secs.map((s,j)=>`<h2 id="w${i}h${j}">${s[0]}</h2>${s[1]}`).join('');
  if(d.tab) h+=`<div class="note">${d.tab.note}</div><div class="tw"><table><tr>${d.tab.head.map(x=>`<th>${x}</th>`).join('')}</tr>${d.tab.rows.map(r=>`<tr>${r.map(x=>`<td>${x}</td>`).join('')}</tr>`).join('')}</table></div>`;
  if(d.t==='images'){const tones=[['oklch(45% .06 240)','Base, north'],['oklch(50% .05 150)','Foundation'],['oklch(42% .04 40)','Segment 2'],['oklch(38% .05 280)','Crane'],['oklch(48% .03 200)','Antenna'],['oklch(36% .02 100)','Gatehouse']];h+=`<div class="note">These images lie in a folder on the disk. This collection only shows them.</div><div class="imgs">${tones.map(t=>`<div data-style="background:${t[0]}">${t[1]}</div>`).join('')}</div>`;}
  if(d.t==='folder') h+=`<p>${d.why}</p>`;
  if(d.kids.length) h+=`${d.t==='folder'||d.t==='coll'?'':'<h2>Below this document</h2>'}<div class="kids">${d.kids.filter(shown).map(k=>`<button data-act="open" data-id="${k}">${ic(D[k].i||TYPE[D[k].t][0])}<span>${D[k].n}</span><em>${TYPE[D[k].t][1]}</em></button>`).join('')}</div>`;
  if(d.html) h=d.t==='coll'?d.html+h:h+d.html;
  return {h,hs};
}
Object.assign(I,{chat:'<path d="M4 5h16v11H9l-5 4z"/>',search:'<circle cx="11" cy="11" r="7"/><path d="m20 20-4-4"/>',
  /* The gear is Lucide's "settings" (ISC, licenses/Lucide-LICENSE.txt): the drawn one read as a sun (2026-11-09). */
  gear:'<path d="M9.671 4.136a2.34 2.34 0 0 1 4.659 0 2.34 2.34 0 0 0 3.319 1.915 2.34 2.34 0 0 1 2.33 4.033 2.34 2.34 0 0 0 0 3.831 2.34 2.34 0 0 1-2.33 4.033 2.34 2.34 0 0 0-3.319 1.915 2.34 2.34 0 0 1-4.659 0 2.34 2.34 0 0 0-3.32-1.915 2.34 2.34 0 0 1-2.33-4.033 2.34 2.34 0 0 0 0-3.831A2.34 2.34 0 0 1 6.35 6.051a2.34 2.34 0 0 0 3.319-1.915"/><circle cx="12" cy="12" r="3"/>',
  pin:'<path d="M9 4h6l-1 6 3 3v2H7v-2l3-3zM12 15v6"/>', more:'<path d="M5 12h.01M12 12h.01M19 12h.01"/>', x:'<path d="M6 6l12 12M18 6 6 18"/>',
  grip:'<path d="M9 6h.01M15 6h.01M9 12h.01M15 12h.01M9 18h.01M15 18h.01"/>', star:'<path d="m12 3 2.8 5.8 6.2.9-4.5 4.4 1 6.3-5.5-3-5.5 3 1-6.3L3 9.7l6.2-.9z"/>',
  code:'<path d="m8 8-4 4 4 4M16 8l4 4-4 4M14 5l-4 14"/>', pulse:'<path d="M3 12h4l3-8 4 16 3-8h4"/>', flag:'<path d="M5 21V4h13l-3 4 3 4H5"/>',
  pencil:'<path d="m4 20 4-1L19 8l-3-3L5 16z"/>', back:'<path d="m15 6-6 6 6 6"/>', layout:'<rect x="3" y="4" width="18" height="16" rx="1"/><path d="M8 4v16M3 10h5M3 15h5"/>', split:'<rect x="3" y="5" width="18" height="14" rx="1"/><path d="M12 5v14"/>',up:'<path d="m6 15 6-6 6 6"/>', down:'<path d="m6 9 6 6 6-6"/>'});
const TAGS_={net:['network'],plan:['network','main-site'],vlans:['network'],ports:['network','hardware'],tickets:['network'],hand:['on-call'],esc:['on-call'],cont:['on-call','people'],tower:['tower','main-site'],dims:['tower'],photos:['tower','main-site'],inv:['hardware'],tickets2:['hardware'],n1:['idea']};
const TAGS=TAGS_;
Object.assign(I,{check:'<rect x="4" y="4" width="16" height="16" rx="2"/><path d="m8 12 3 3 5-6"/>',board:'<rect x="3" y="4" width="18" height="16" rx="1"/><path d="M9 4v16M15 4v16"/>',
  video:'<rect x="3" y="5" width="18" height="14" rx="1"/><path d="m10 9 5 3-5 3z"/>',slides:'<rect x="3" y="4" width="18" height="12" rx="1"/><path d="M12 16v4M8 20h8"/>',
  note:'<path d="M5 4h14v11l-5 5H5z"/><path d="M14 20v-5h5"/>',mail:'<rect x="3" y="5" width="18" height="14" rx="1"/><path d="m3 7 9 6 9-6"/>',
  net:'<rect x="9" y="3" width="6" height="5" rx="1"/><rect x="3" y="16" width="6" height="5" rx="1"/><rect x="15" y="16" width="6" height="5" rx="1"/><path d="M12 8v4M6 16v-4h12v4"/>',send:'<path d="M21 3 3 10l7 3 3 7z"/>',cal:'<rect x="3" y="5" width="18" height="16" rx="1"/><path d="M3 10h18M8 3v4M16 3v4"/>',clock:'<circle cx="12" cy="12" r="9"/><path d="M12 7v5l3 2"/>',sun:'<circle cx="12" cy="12" r="4"/><path d="M12 3v2M12 19v2M3 12h2M19 12h2M5.6 5.6 7 7M17 17l1.4 1.4M5.6 18.4 7 17M17 7l1.4-1.4"/>',globe:'<circle cx="12" cy="12" r="9"/><path d="M3 12h18M12 3c3 3 3 15 0 18M12 3c-3 3-3 15 0 18"/>',grid:'<rect x="4" y="4" width="7" height="7"/><rect x="13" y="4" width="7" height="7"/><rect x="4" y="13" width="7" height="7"/><rect x="13" y="13" width="7" height="7"/>',lock:'<rect x="5" y="11" width="14" height="10" rx="1"/><path d="M8 11V7a4 4 0 0 1 8 0v4"/>',shield:'<path d="M12 3 4 6v6c0 5 3.5 8 8 9 4.5-1 8-4 8-9V6z"/><path d="m9 12 2 2 4-4"/>',opennew:'<path d="M14 4h6v6M20 4l-9 9"/><path d="M18 14v6H4V6h6"/>',quote:'<path d="M9 7H5v6h4zM19 7h-4v6h4zM9 13c0 2-1 4-3 4M19 13c0 2-1 4-3 4"/>',spark:'<path d="M12 3l2 7 7 2-7 2-2 7-2-7-7-2 7-2z"/>',ai:'<path d="M12 3l2 7 7 2-7 2-2 7-2-7-7-2 7-2z"/><path d="M19 3v4M17 5h4"/>',user:'<circle cx="12" cy="8" r="4"/><path d="M4 21c1-5 4-7 8-7s7 2 8 7"/>',plus:'<path d="M12 5v14M5 12h14"/>',eye:'<path d="M2 12s4-7 10-7 10 7 10 7-4 7-10 7-10-7-10-7z"/><circle cx="12" cy="12" r="3"/>'});
Object.assign(TYPE,{todo:['check','To-do list'],kanban:['board','Kanban board'],video:['video','Video'],slides:['slides','PowerPoint'],notes:['note','Quick notes']});
const none={uses:{ext:[],int:[],data:[],vals:[],db:[]},usedIn:[]};
D.ttodo={t:'todo',n:'Tower tasks',kids:[],path:'not a file, lives in the database',by:'J. Roth',ch:'today',vis:'Project team, inherited',...none,
  items:[{t:'Order the second segment',done:true},{t:'Book the crane',done:true},{t:'Inspect the foundation',done:false},{t:'Mount the antenna',done:false},{t:'Update the network plan',done:false}],
  get stat(){return `${this.items.filter(x=>x.done).length} of ${this.items.length} done`;},
  get html(){return `<div class="todos">${this.items.map((x,k)=>`<button class="todo${x.done?' done':''}" data-act="todo" data-id="${k}" aria-pressed="${x.done}">${x.done?ic('check'):'<span class="box"></span>'}<span>${x.t}</span></button>`).join('')}</div>`;}};
D.tboard={t:'kanban',n:'Tower board',w:'full',kids:[],path:'not a file, lives in the database',by:'J. Roth',ch:'today',vis:'Project team, inherited',...none,stat:'6 cards in 3 columns',
  html:`<div class="note">A board is another view of a list, so the to-do list above could show like this too. Dragging cards comes with M-6.</div><div class="board">${[['To do',['Mount the antenna','Update the network plan']],['Doing',['Inspect the foundation','Paint the top segment']],['Done',['Order the second segment','Book the crane']]].map(c=>`<section><h3>${c[0]} <em>${c[1].length}</em></h3>${c[1].map(x=>`<div class="kard">${x}</div>`).join('')}</section>`).join('')}</div>`};
D.walk={t:'video',n:'Site walkthrough',w:'wide',kids:[],path:'operations/network/walkthrough.mp4',by:'A. Keller',ch:'Oct 12',vis:'Group ops, inherited',...none,stat:'12:34 · 48 MB',
  html:`<div class="vid" role="img" aria-label="Video player">${ic('video')}<span>0:00 / 12:34</span></div><p class="lab" data-style="margin-top:8px">The file lies on the share. The browser plays it, nothing is copied.</p>`};
D.deck={t:'slides',n:'Kickoff slides',w:'wide',cur:0,kids:[],path:'projects/tower/kickoff.pptx',by:'J. Roth',ch:'Sep 30',vis:'Project team, inherited',...none,
  titles:['Tower 50 m','Why we build','Schedule','Open questions'],
  get stat(){return `Slide ${this.cur+1} of ${this.titles.length}`;},
  get html(){return `<div class="slide">${this.titles[this.cur]}</div><div class="thumbs">${this.titles.map((x,k)=>`<button class="thumb" data-act="slide" data-id="${k}" aria-pressed="${k===this.cur}"><b>${k+1}</b>${x}</button>`).join('')}</div><p class="lab" data-style="margin-top:8px">Read-only preview. To edit, open the file in PowerPoint or LibreOffice over the share.</p>`;}};
D.qn={t:'notes',n:'Quick notes',w:'full',kids:[],path:'not a file, private notes',by:'you',ch:'today',vis:'Only you',...none,
  notes:[{h:'Call the crane company',b:'Ask for the Tuesday slot.'},{h:'',b:'Spare antenna: check the shelf in the workshop.'},{h:'Shopping',b:'Cable ties\nLabels\nGloves'},{h:'Idea',b:'Put the tower height on the start page.'},{h:'',b:'Rack photo is out of date.'}],
  get stat(){return `${this.notes.length} notes`;},
  get html(){return `<input class="noteadd" data-addnote placeholder="Take a note, then Enter" aria-label="Take a note"><div class="keep">${this.notes.map(n=>`<div class="kard">${n.h?`<b>${esc(n.h)}</b>`:''}<span>${esc(n.b).replace(/\n/g,'<br>')}</span></div>`).join('')}</div>`;}};
D.tower.kids=[...D.tower.kids,'ttodo','tboard','deck'];
D.net.kids=[...D.net.kids,'walk'];
C.find(c=>c.id==='me').top=['qn','n1'];
Object.assign(TAGS,{ttodo:['tower'],tboard:['tower'],deck:['tower'],walk:['network'],qn:['idea']});
D.plan.desc='Router, core switches and the radio link of the main site.';D.plan.cby='J. Roth';D.plan.created='Sep 12';D.tower.desc='Build of the 50 m tower: status, dimensions and photos.';D.hand.desc='What to do when something breaks at night.';
D.tower.doc.intro='This page stores the width full, so it uses all the room its window has. The navigation stays where it is.';
D.hand.toc='side';
D.old={t:'page',n:'Old offer notes',w:'slim',kids:[],path:'archive/old-offer.md',by:'A. Keller',ch:'Mar 04',vis:'Group ops',uses:{ext:[],int:[],data:[],vals:[],db:[]},usedIn:[],doc:P('Kept for reference.',['Result','<p>Not ordered.</p>'],['Reason','<p>Too expensive.</p>'])};
Object.entries(D).forEach(([k,d])=>{d.w=d.w||'slim';d.toc=d.toc||'marker';d.tags=TAGS[k]||[];});
const ICON={ops:'pulse',pro:'flag',me:'pencil',arc:'box'};
C.push({id:'arc',n:'Archive',top:['old']});
C.forEach(c=>c.i=ICON[c.id]);
/* A collection is an object like a document: it has an ID, opens in a window with its overview, and can sit inside another collection. */
TYPE.coll=['grid','Collection'];TYPE.line=['grid','Line'];TYPE.space=['grid','Space'];
const isDeco=id=>!!D[id]&&(D[id].t==='line'||D[id].t==='space');
/* The container of an entry of nav3: its parent, or the collection it lies in at the top. */
const contOf=id=>PARENT[id]||(COLL[id]&&COLL[id].id);
/* Made-up content changes only in this browser. Objects of the database change there, and only where one may edit;
   the folder source is read only. */
const canPlace=cont=>!!D[cont]&&(D[cont].db?accOf(cont)==='edit':!D[cont].real);
const placeWhy=cont=>D[cont]&&D[cont].real&&!D[cont].db?'The folder source is read only':ACC[accOf(cont)]||'Not here';
function divItems(mode,id){const cont=mode==='first'?id:contOf(id),ok=canPlace(cont)?'':'disabled title="'+placeWhy(cont)+'"';
  return '<hr>'+`<button data-act="line${mode}" data-id="${id}" ${ok}>${ic('hr')}Line ${mode==='first'?'at the top of it':'below it'}</button><button data-act="space${mode}" data-id="${id}" ${ok}>${ic('gap')}Space ${mode==='first'?'at the top of it':'below it'}</button>`;}
function collDoc(id,n,kids,i){D[id]={t:'coll',n,i,kids,w:'wide',toc:'marker',tags:[],path:'not a file, a collection',by:'A. Keller',ch:'Oct 31',vis:'Group ops',...none,
  get stat(){return `${this.kids.length} entries`;},
  /* The overview holds the settings of the collection too, for whoever may change them (the maintainer, 2026-11-09). */
  get html(){const ac=accOf(id),may=this.db?ac==='edit'||!!this.mine:!this.real&&ac==='edit',top=C.some(c=>c.id===id&&!c.alias),owner=isOwner(id);
    const facts=`<dl class="mailhead colfacts"><dt>Owner</dt><dd>${this.owner||'A. Keller'}</dd>${may?'':`<dt>Others can</dt><dd>${ACC[ac].toLowerCase()}${this.acc?'':' (the default)'}</dd>`}<dt>Visible to</dt><dd>${id==='me'||this.private?'only you':'everyone signed in'}</dd><dt>Entries</dt><dd>${this.kids.length}</dd></dl>`;
    const set=may?`<h2>Settings</h2><label class="slab" for="cname">Name</label><input id="cname" class="addtag" data-cname="${id}" value="${this.n}" aria-label="Name of the collection">
      <div class="slab">Others can</div><span class="seg" role="group">${(top?[]:[['inherit','As above']]).concat([['edit','Edit'],['comment','Comment'],['read','Read']]).map(x=>`<button data-act="cacc" data-c="${id}" data-id="${x[0]}" aria-pressed="${(this.acc||(top?'edit':'inherit'))===x[0]}"${owner?'':' disabled title="Only the owner sets this"'}>${x[1]}</button>`).join('')}</span>
      <p class="hint">Inherited by everything in it, unless something sets its own. The owner can always edit, and only the owner sets this.</p>`:'';
    return facts+set+(this.kids.length?'':'<p class="lab">Empty. New + at its heading in nav3 adds something.</p>');}};}
/* A collection in nav2 reads its name and list from its object, so moving it between nav2 and nav3 keeps one object. */
function asTop(c){Object.defineProperty(c,'n',{get(){return D[c.id].n;},set(v){D[c.id].n=v;},configurable:true,enumerable:true});Object.defineProperty(c,'top',{get(){return D[c.id].kids;},configurable:true,enumerable:true});return c;}
C.forEach(c=>{const n=c.n,top=c.top;delete c.n;delete c.top;collDoc(c.id,n,top,c.i);asTop(c);});
D.bud={t:'page',n:'Budget 2027',w:'slim',toc:'marker',tags:[],kids:[],path:'projects/planning-2027/budget.md',by:'J. Roth',ch:'Nov 02',vis:'Project team, inherited',...none,doc:P('First numbers for next year.',['Hardware','<p>Two switches and a spare antenna.</p>'],['Services','<p>The crane for one day.</p>'])};
collDoc('plan27','Planning 2027',['bud'],'grid');D.plan27.desc='Ideas and numbers for next year. A collection inside Projects.';
C.find(c=>c.id==='pro').top.push('plan27');
/* Collections inside a collection below Projects, to try showing one in nav2 (2026-11-09). */
collDoc('bud27','Budget lines',[],'grid');D.plan27.kids.push('bud27');collDoc('q127','Quarter 1',[],'grid');D.bud27.kids.push('q127');
D.plan27.i='grid';
D.ops.desc='Network, site logs and the on-call handbook.';D.arc.acc='read';D.hand.acc='comment';
TYPE.coll[0]='grid';
D.pro.desc='Builds and purchases.';
const PARENT={},COLL={};
/* A collection inside another can be shown in nav2 as well (the maintainer, 2026-11-09). That entry only points at it: the
   collection lies where it lies, the tree takes no notice of the entry, and the entry goes when the collection stops
   being one, or comes to stand in nav2 itself. */
const aliasOf=id=>C.some(c=>c.alias&&c.id===id);
function asAlias(id){return {id,alias:true,get n(){return D[id].n;},get top(){return D[id].kids;},get i(){return D[id].i||'grid';}};}
function pruneAliases(){for(let i=C.length-1;i>=0;i--){const c=C[i];if(c.alias&&(!D[c.id]||D[c.id].t!=='coll'||C.some(x=>!x.alias&&x.id===c.id)))C.splice(i,1);}}
function rebuildPlaces(){for(const k in PARENT)delete PARENT[k];for(const k in COLL)delete COLL[k];pruneAliases();
  C.forEach(c=>{if(c.alias)return;const walk=(id,p)=>{PARENT[id]=p;COLL[id]=c;D[id].kids.forEach(k=>walk(k,id));};c.top.forEach(id=>walk(id,null));});}
rebuildPlaces();
const placeOf=id=>{if(D[id].place)return D[id].place;if(C.some(c=>c.id===id))return 'Workspace';const a=[];for(let x=PARENT[id];x;x=PARENT[x])a.unshift(D[x].n);return (COLL[id]?[COLL[id].n,...a]:a).join(' › ');};
const esc=s=>String(s).replace(/[&<>"]/g,m=>({'&':'&amp;','<':'&lt;','>':'&gt;','"':'&quot;'}[m]));
const plain=h=>{const e=document.createElement('div');e.innerHTML=h;return e.textContent;};
const phone=()=>matchMedia('(max-width: 760px)').matches;
const W3=300;
const newTab=(d,general=false)=>({d,general,top:0,fly:false,gear:false,cmt:false,find:false,q:'',cur:0,src:false});
const S={help:false,user:{name:'',login:'',email:'',groups:[],initials:'?',admin:false},view:'workspace',prev:'workspace',sec:{files:'share',resources:'values',mail:'inbox',bookmarks:'bar',news:'all',profile:'acct',todo:'mine',cal:'all',time:'week',vac:'y26',notes:'quick'},landing:false,cq:'',addIn:null,just:null,prof:false,filters:false,newN:0,
  focus:0,rail:!phone(),limit:8,tog:{},treeTop:0,s1:0,s2:3,w3:W3,theme:'system',scale:100,first:false,clock:false,
  ctx:null,newAt:null,tmenu:null,hist:null,tlist:null,toast:'',aid:false,chat:[],into:'qn',
  fav:{ops:true,pro:true,me:true,arc:false},open:{ops:true,pro:true},pin:{ops:true},so:{todo:{mine:true},cal:{all:true},time:{week:true},vac:{y26:true},notes:{quick:true}},spin:{},mods:{mail:true,private:true,bookmarks:true,news:true,ai:true,network:true,lists:true,video:true,slides:true,notes:true},menu:null,
  sq:'',strat:'full',ai:false,ftype:'',ftags:[],
  wins:[{tabs:[newTab('plan',true)],a:0,hist:['plan']}]};
const at=i=>S.wins[i].tabs[S.wins[i].a]||null;
/* What others may do: edit, comment or only read. Set on a collection, folder or document, and inherited by what is below it. */
const ACC={edit:'Can edit',comment:'Can comment',read:'Read only'};
const contAcc=id=>PARENT[id]?accOf(PARENT[id]):COLL[id]?accOf(COLL[id].id):'edit';
const accOf=id=>{for(let x=id;x;x=PARENT[x])if(D[x]&&D[x].acc)return D[x].acc;const c=COLL[id];return c&&D[c.id].acc||'edit';};
const kidsOpen=id=>{const v=S.tog[id];return v!==undefined?v:id.startsWith('dir:')?false:D[id].kids.length<=S.limit;};
const chg=d=>String(d.ch).includes(' by ')?d.ch:`${d.ch} by ${d.by||'–'}`;
const tagChips=(tags,act)=>tags.map(t=>`<button class="tagc" data-act="${act}" data-tag="${t}" title="All objects tagged ${t}">#${t}</button>`).join('');

function rowHtml(id){
  if(!shown(id))return '';
  const sl=isSel('n3',id)?' aria-selected="true"':'';
  if(isDeco(id))return `<li><div class="row deco" data-deco="${D[id].t}" data-row="${id}"${sl} title="${D[id].n}: drag it, or right click to remove it"></div></li>`;
  const d=D[id],open=kidsOpen(id),t=at(S.focus),cur=t&&t.d===id;
  return `<li><div class="row${cur?' cur':''}${S.just===id?' just':''}" data-row="${id}"${sl}>
    <button class="arrow ${d.kids.length?(open?'open':''):'none'}" data-act="tog" data-id="${id}" aria-label="Toggle ${d.n}">${ic('chev')}</button>
    ${S.ren===id?`<span class="name">${ic(d.i||TYPE[d.t][0])}<input class="renin" data-ren="${id}" value="${d.n}" aria-label="Name"></span>`:`<button class="name" data-act="open" data-id="${id}" title="${d.n}">${ic(d.i||TYPE[d.t][0])}<span>${d.n}</span></button>`}</div>
    ${d.kids.length&&open?`<ul class="t">${d.kids.map(rowHtml).join('')}</ul>`:''}</li>`;
}
const ADDTYPES=['page','folder','bookmark','search','table','todo','kanban','notes','excel','slides','video','images','view','db','ipnet','devices'];
/* The first build: switched-off modules disappear, but what is not built yet stays in sight, greyed out. */
const LATER={view:['files','resources','bookmarks','news','mail','todo','cal','time','vac','notes'],add:ADDTYPES.filter(k=>!['page','folder'].includes(k)),prof:['lang','notif','mods','src','store','audit'],tool:['split','opencol','newwin','aid','qn']};
const later=(kind,id)=>S.first&&LATER[kind].includes(id);
const off=(kind,id)=>later(kind,id)?' disabled data-later="1"':'';
const addTypes=()=>ADDTYPES.filter(k=>!MODTYPE[k]||S.mods[MODTYPE[k]]);
function treeHtml(){
  const open=openColls();
  if(!open.length) return '<p class="lab pad">No collection is open. Pick one in nav2.</p>';
  return open.map((c,k)=>`<div class="cheadrow${k?'':' first'}" data-cdrop="${c.id}" data-ctx="coll" data-ctxid="${c.id}"><button class="chead" data-act="open" data-id="${c.id}" title="Open ${c.n}">${c.n}${accOf(c.id)!=='edit'?`<span class="acc" title="${ACC[accOf(c.id)]}">${ic('lock')}</span>`:''}</button><button class="addbtn newplus" data-act="addmenu" data-id="${c.id}" aria-expanded="${S.addIn===c.id}" aria-label="New in ${c.n}" title="${accOf(c.id)==='edit'?'New in '+c.n:ACC[accOf(c.id)]}"${accOf(c.id)==='edit'?'':' disabled'}>New ${ic('plus')}</button></div>
    <ul class="t top">${c.top.map(rowHtml).join('')}</ul>`).join('');
}
/* New, in nav2 and in the head of a collection: one menu in groups, and typing filters it (the command menu of C22).
   It floats over everything below its button, so nothing moves. */
Object.assign(I,{link:'<path d="M10 14a4 4 0 0 0 5.7 0l3-3a4 4 0 0 0-5.7-5.7l-1 1"/><path d="M14 10a4 4 0 0 0-5.7 0l-3 3a4 4 0 0 0 5.7 5.7l1-1"/>',hr:'<path d="M4 12h16"/>',gap:'<path d="M4 5h16M4 19h16M12 9v6"/>'});
const SOON=' disabled data-later="1" title="Comes later"';
/* New in a collection: what is needed most comes first, a page, then tables, databases, single records, and boards and
   lists (the maintainer, 2026-11-09); within a block the same order of need. */
const NEWGROUPS=[['',['page']],['Tables',['table','excel']],['Databases',['db','view']],['Records',['ipnet','devices','bookmark']],['Boards and lists',['kanban','todo','notes']],['Media',['images','slides','video']],['Organize',['folder','search']]];
function addPop(){
  if(!S.addIn)return '';
  const n2=S.addIn==='n2',c=!n2&&D[S.addIn];if(!n2&&!c)return '';
  const it=(act,id,label,icon,extra='')=>`<button role="menuitem" data-act="${act}" data-id="${id}" data-filter="${label}" ${extra}>${ic(icon)}<span>${label}</span></button>`;
  const grp=(h,items)=>`<div role="group">${h?`<span role="heading">${h}</span>`:''}${items}</div>`;
  const place=n2?'':canPlace(S.addIn)?'':' disabled title="'+placeWhy(S.addIn)+'"';
  const groups=n2?[grp('Make',it('newcoll','','Collection','grid')),grp('Set apart',it('n2line','','Line','hr')+it('n2space','','Space','gap')),grp('Bring in',it('none','','Import','down',SOON)+it('none','','Link','link',SOON))]
    :[...NEWGROUPS.map(([h,ks])=>{const items=ks.filter(k=>addTypes().includes(k)).map(k=>[TYPE[k][1],it('add',k,TYPE[k][1],TYPE[k][0],`data-c="${S.addIn}"`+off('add',k))]);
        if(h==='Organize')items.push(['Collection',it('newcoll',S.addIn,'Collection','grid')]);
        return items.length?grp(h,items.map(x=>x[1]).join('')):'';}).filter(Boolean),
      grp('Set apart',it('lineend',S.addIn,'Line','hr',place)+it('spaceend',S.addIn,'Space','gap',place)),
      grp('Bring in',it('none','','Import','down',SOON)+it('none','','Link','link',SOON)+(S.mods.bookmarks?it('open','bm:import','Import bookmarks','down',off('add','bookmark')):''))];
  return `<div class="pop ctxpop cmdpop" data-style="left:${S.addAt.x}px;top:${S.addAt.y}px"><div class="command" aria-label="New ${n2?'in nav2':'in '+c.n}"><header>${ic('search')}<input data-nfilter value="${esc(S.addQ)}" placeholder="Type to filter" aria-label="Filter" autocomplete="off" spellcheck="false" role="combobox" aria-expanded="true"></header><div role="menu" data-empty="Nothing matches.">${groups.join('<hr role="separator">')}</div></div></div>`;
}
/* Typing filters the entries by their names; arrows move the choice, Enter takes it. The focus stays in the field. */
function filterNew(q){
  const items=[...document.querySelectorAll('.cmdpop [role="menuitem"]')],w=q.trim().toLowerCase();
  items.forEach(n=>{n.classList.remove('active');n.setAttribute('aria-hidden',String(!!w&&!n.dataset.filter.toLowerCase().includes(w)));});
  const first=items.find(n=>n.getAttribute('aria-hidden')!=='true'&&!n.disabled);if(first&&w)first.classList.add('active');
}
function stepNew(dir){
  const items=[...document.querySelectorAll('.cmdpop [role="menuitem"]')].filter(n=>n.getAttribute('aria-hidden')!=='true'&&!n.disabled);if(!items.length)return;
  const k=items.findIndex(n=>n.classList.contains('active'));items.forEach(n=>n.classList.remove('active'));
  const n=items[(k<0?(dir>0?0:items.length-1):k+dir+items.length)%items.length];n.classList.add('active');n.scrollIntoView({block:'nearest'});
}
/* A new collection gets the first free number, and its name is written in place. */
function freeName(base){const used=new Set(Object.values(D).filter(d=>d.t==='coll'||d.t==='folder').map(d=>plain(d.n)));let n=1;while(used.has(`${base} (${n})`))n++;return `${base} (${n})`;}
/* Every entry of a menu carries its icon (2026-11-09). */
Object.assign(I,{trash:'<path d="M4 7h16M9 7V4h6v3M6 7l1 13h10l1-13M10 11v6M14 11v6"/>',copy:'<rect x="8" y="8" width="12" height="12" rx="2"/><path d="M16 8V5a1 1 0 0 0-1-1H5a1 1 0 0 0-1 1v10a1 1 0 0 0 1 1h3"/>',
  sort:'<path d="M7 4v16M4 17l3 3 3-3M14 6h6M14 12h4M14 18h2"/>',win:'<rect x="3" y="4" width="18" height="16" rx="2"/><path d="M3 9h18"/>'});
const CTXI={copen:'eye',cpin:'pin',collhead:'x',open:'eye',addmenu2:'plus',newcoll:'grid',n2line:'hr',n2space:'gap',linefirst:'hr',lineafter:'hr',lineend:'hr',spacefirst:'gap',spaceafter:'gap',spaceend:'gap',
  cgear:'gear',opengear:'gear',decodel:'trash',opentab:'layout',opencol:'split',addchild:'plus',copylink:'link',sec:'eye',secclose:'x',secup:'up',secdown:'down',none:'pencil',view:'eye',
  selopen:'layout',selpin:'pin',selclear:'x',dupl:'copy',sortaz:'sort',sortza:'sort',close:'x',closeothers:'x',closeright:'x',closeleft:'x',closeall:'x',keep:'pin',dup:'copy',copypath:'link',
  newwin:'split',move:'split',closewin:'x',aid:'ai',split:'split',delobj:'trash',trash:'trash'};
const mi=act=>ic(CTXI[act]||'more');
/* A right click opens the menu of what is under it, everywhere: tab, collection, list entry, section, view. */
/* Duplicate, where one may add next to it, and sort the children of anything that holds two or more. */
/* Turn a collection into a folder or back. A collection at the top, and the private notes, stay collections. */
/* Show a collection that lies inside another in nav2 too, or no longer. */
function showItem(id){
  if(!D[id]||D[id].t!=='coll'||C.some(c=>!c.alias&&c.id===id))return '';
  return aliasOf(id)?`<button data-act="unshow" data-id="${id}">${ic('x')}Remove from nav2</button>`:`<button data-act="show" data-id="${id}">${ic('pin')}Show in nav2</button>`;
}
function kindItem(id){
  const d=D[id];if(d.t!=='coll'&&d.t!=='folder')return '';if(d.real&&!d.db)return '';
  if(d.t==='coll'&&(C.some(c=>c.id===id&&!c.alias)||d.private))return '';
  const may=d.db?accOf(id)==='edit':true,to=d.t==='coll'?'folder':'collection';
  return `<button data-act="tokind" data-id="${id}" data-k="${to}"${may?'':' disabled'}>${ic(to==='folder'?'folder':'grid')}Turn into a ${to}</button>`;
}
/* Delete sits last, after a line, and asks in place: the first click turns it into the question, the second deletes
   (no confirm(), as in C22). What lies in the database goes into the trash with everything below it; made-up content
   goes here and now. Whoever may change it may delete it, a collection at the top only its owner, and private notes stay. */
function delItem(id){
  const d=D[id];if(!d||isDeco(id))return '';
  const top=C.some(c=>c.id===id&&!c.alias),ask=!!(S.ctx&&S.ctx.ask);
  const why=d.real&&!d.db?'The folder source is read only':d.private||id==='me'?'Private notes stay':!d.db?'':top&&!d.mine?'Only its owner deletes a collection at the top':!d.mine&&accOf(id)!=='edit'?ACC[accOf(id)]:'';
  return `<hr><button data-act="delobj" data-id="${id}" aria-pressed="${ask}"${why?` disabled title="${why}"`:''}>${mi('delobj')}${ask?'Delete? Yes':'Delete'}</button>`;
}
function dupSort(id){
  const cont=contOf(id),may=cont?canPlace(cont):(D[id].db?S.canCreate:!D[id].real),kids=D[id].kids.filter(x=>!isDeco(x)).length,sorts=kids>1&&(D[id].db?accOf(id)==='edit':!D[id].real);
  return `<hr><button data-act="dupl" data-id="${id}"${may?'':' disabled'}>${mi('dupl')}Duplicate</button>`+(sorts?`<button data-act="sortaz" data-id="${id}">${mi('sortaz')}Sort its children A to Z</button><button data-act="sortza" data-id="${id}">${mi('sortza')}Sort its children Z to A</button>`:'');
}
function ctxHtml(){
  const c=S.ctx;if(!c)return '';
  const B=(act,label,id=c.id,extra='')=>`<button data-act="${act}" data-id="${id}" ${extra}>${mi(act)}${label}</button>`;let items='',title='';
  if(c.kind==='coll'){title=D[c.id].n;items=B('copen','Open, close what is not kept open')+B('cpin',S.pin[c.id]?'Stop keeping it open':'Keep it open',c.id,'title="Also a double click"')+(S.open[c.id]?B('collhead','Close'):'')+B('open','Open its overview')+'<hr>'+B('addmenu2','New in it')+B('newcoll','New collection inside it')+(c.n3?divItems('first',c.id):'<hr>'+B('n2line','Line below it')+B('n2space','Space below it'))+showItem(c.id)+dupSort(c.id)+kindItem(c.id)+'<hr>'+B('cgear','Settings and name')+delItem(c.id);}
  if(c.kind==='deco'){title=D[c.id].n;items=B('decodel','Remove');}
  if(c.kind==='win'){const w=S.wins[0],older=olderOf(w);title='Window';items=tabMenuHtml(0,w.a)+(older.length?'<hr>'+older.map(x=>`<button data-act="open" data-id="${x}">${ic(TYPE[D[x].t][0])}${D[x].n}</button>`).join(''):'')+(S.mods.ai?'<hr>'+B('aid','AI assistant','',off('tool','aid')):'');}
  if(c.kind==='sel'&&S.sel){const z=S.sel.z;title=S.sel.ids.length+' selected';items=(z==='n3'?B('selopen','Open as tabs'):z==='n2'?B('selopen','Open them in nav3')+B('selpin','Keep them open'):'')+'<hr>'+B('selclear','Clear the selection');}
  if(c.kind==='row'){const d=D[c.id];title=d.n;items=B('open','Open')+B('opentab','Open as a tab of its own')+(S.wins.length<4?B('opencol','Open in a new column',c.id,off('tool','opencol')):'')+'<hr>'+(COLL[c.id]?B('addchild','New page below it',c.id,accOf(c.id)==='edit'?'':'disabled title="'+ACC[accOf(c.id)]+'"'):'')+divItems('after',c.id)+B('copylink','Copy link')+showItem(c.id)+dupSort(c.id)+kindItem(c.id)+B('opengear','Settings and metadata')+delItem(c.id);}
  if(c.kind==='row'&&isDeco(c.id)){title=D[c.id].n;items=B('decodel','Remove',c.id,canPlace(contOf(c.id))?'':'disabled title="'+placeWhy(contOf(c.id))+'"');}
  if(c.kind==='sec'){title=c.label;items=B('sec','Open')+(S.so[S.view]&&S.so[S.view][c.id]?B('secclose','Close'):'')+(c.sort?B('secup','Move up')+B('secdown','Move down'):'')+'<hr>'+B('none','Rename','','disabled title="Comes with editing"');}
  if(c.kind==='free'){title='New';items=c.acts.map(x=>{if(x[0]==='-')return '<hr>';const [act,id]=x[0].split(':');return B(act,x[1],id||'');}).join('');}
  if(c.kind==='view'){title=c.label;items=B('view','Open')+(S.view===c.id&&S.rail?B('view','Hide nav2 and nav3'):'');}
  return `<div class="pop ctxpop" role="menu" data-style="left:${c.x}px;top:${c.y}px"><b>${title}</b>${items}</div>`;
}
function navItem(act,id,label,icon,pressed,extra='',no=false){const l=no||(act==='view'&&later('view',id))||(act==='sec'&&S.view==='profile'&&later('prof',id));return `<button class="navitem" data-act="${act}" data-id="${id}" aria-pressed="${pressed}" title="${label}${l?', not in the first build':''}" ${extra}${l?' disabled data-later="1"':''}>${ic(icon)}<span class="lbl">${label}</span></button>`;}
/* What nav3 shows: the open collections of nav2, in its order, those shown there from inside others among them. */
const openColls=()=>C.filter(c=>!isDeco(c.id)&&S.open[c.id]);
function nav2Html(){
  const s=phone()?1:S.s2;
  if(S.view==='workspace'){
    /* Every collection is in nav2, so an open one is always in sight: favorites on top, the others below. A right click has the rest. */
    /* Open for now or kept open: kept open draws the line on the left. Lines and spaces set groups apart; all of it is
       sorted by dragging, and the account keeps the order (the browser a copy). */
    const sl=c=>isSel('n2',c.id)?' aria-selected="true"':'';
    const row=c=>isDeco(c.id)?`<div class="collrow deco" data-deco="${D[c.id].t}" data-favid="${c.id}"${sl(c)} data-ctx="deco" data-ctxid="${c.id}" title="${D[c.id].n}: drag it, or right click to remove it"></div>`
      :S.ren===c.id?`<div class="collrow ren">${ic(c.i||'grid')}<input class="renin" data-ren="${c.id}" value="${D[c.id].n}" aria-label="Name of the collection"></div>`
      :`<div class="collrow" data-ctx="coll" data-ctxid="${c.id}" data-favid="${c.id}"${sl(c)}${c.alias?` data-alias="1" title="Lies in ${placeOf(c.id)}, shown here too"`:''}>${navItem('coll',c.id,c.n,c.i||D[c.id].i||'grid',!!S.open[c.id],S.pin[c.id]?'data-pinned="1"':'')}</div>`;
    return `<div class="n2head"><span class="lbl">Collections</span><button class="addbtn newplus" data-act="addmenu" data-id="n2" aria-expanded="${S.addIn==='n2'}" aria-label="New in nav2" title="New: a collection, a line, a space"><span class="nt">New</span> ${ic('plus')}</button></div><div id="clist">${C.map(row).join('')}</div>`;
  }
  if(S.view==='search') return [['exact','Exact words','quote'],['full','Full text','search'],['meaning','Similar meaning','spark']].map(x=>navItem('strat',x[0],x[1],x[2],S.strat===x[0])).join('')+`<span class="favsep"></span>`+navItem('ai','ai','Also ask AI','ai',S.ai);
  if(S.view==='profile') return PROFSET.map(x=>navItem('sec',x[0],x[1],x[2],S.sec.profile===x[0])).join('')+`<span class="favsep"></span><span class="n2lab">${ic('shield')}<span class="lbl">Administration</span></span>`+PROFADM.map(x=>navItem('sec',x[0],x[1],x[2],S.sec.profile===x[0])).join('');
  /* Lists that belong to the user can be sorted by dragging. Entries that act, such as import, sit apart below them. */
  const all=SECTIONS[S.view](),srt=!!SORT[S.view],items=all.filter(x=>!x[3]),acts=all.filter(x=>x[3]);
  const so=S.so[S.view],sp=S.spin[S.view]||{};
  const one=x=>navItem('sec',x[0],x[1],x[2],so&&!x[3]?!!so[x[0]]:S.sec[S.view]===x[0],srt&&!x[3]&&!x[4]?`data-secid="${x[0]}"${isSel('sec',x[0])?' aria-selected="true"':''}`:'');const one2=x=>{const h=one(x);return sp[x[0]]?h.replace('<button class="navitem"','<button class="navitem" data-pinned="1"'):h;};
  return items.map(one2).join('')+(acts.length?'<span class="favsep"></span>'+acts.map(one).join(''):'');
}
const SECTIONS={
  files:()=>[['home','My files','user'],['share','Share','hd'],['ext','Add a source','plus',1]],
  todo:()=>[...TL.map(l=>[l.id,l.n,l.i]),['newlist','New list','plus',1]],
  cal:()=>[['all','All calendars','grid',0,1],...CALS.map(c=>[c.id,c.n,c.i]),['import','Import a calendar','down',1]],
  time:()=>[['week','This week','clock'],['last','Last week','clock'],['oct','October','cal'],['export','Export','down',1]],
  vac:()=>[['y26','2026','sun'],['y27','2027','sun'],['plan','Plan a vacation','plus',1]],
  notes:()=>[...BOARDS.map(b=>[b.id,b.n,b.i]),['newboard','New board','plus',1]],
  resources:()=>RES.map(g=>[g.id,g.n,g.i]),
  mail:()=>[['inbox','Inbox','mail'],['sent','Sent','send'],['archive','Archive','box']],
  bookmarks:()=>[...BM.map(f=>[f.id,f.n,f.i]),['import','Import','down',1]],
  news:()=>[...FEEDS.map(f=>[f.id,f.n,f.i]),['sub','Subscribe','plus',1]],
};
const docText=d=>[d.n,d.tags.join(' '),d.doc?d.doc.intro:'',d.doc?d.doc.secs.map(s=>s[0]+' '+plain(s[1])).join(' '):'',d.tab?d.tab.rows.flat().join(' '):'',d.html?plain(d.html):''].join(' ');
function hits(){
  const q=S.sq.trim();
  if(!q&&!S.ftags.length&&!S.ftype) return null;
  return Object.keys(D).filter(id=>{const d=D[id];if(id.includes(':')||d.gone)return false;
    if(S.ftype&&d.t!==S.ftype) return false;
    if(!S.ftags.every(t=>d.tags.includes(t))) return false;
    if(!q) return true;
    return S.strat==='exact'?docText(d).includes(q):docText(d).toLowerCase().includes(q.toLowerCase());});
}
/* The full-text search of the server finds pages that are not loaded yet; its hits come first. */
const hitsWithServer=()=>{const local=hits();const q=S.sq.trim();if(local===null||!q||S.apiQ!==q||!S.apiHits)return local;
  const real=S.apiHits.map(h=>SID[h.id]).filter(id=>D[id]&&!local.includes(id)&&(!S.ftype||D[id].t===S.ftype));return [...real,...local];};
let searchTimer=null;
function searchServer(){clearTimeout(searchTimer);const q=S.sq.trim();if(!q)return;searchTimer=setTimeout(async()=>{
  try{const r=await fetch('/api/search?q='+encodeURIComponent(q),{credentials:'same-origin',headers:{Accept:'application/json'}});if(!r.ok)return;
    const hs=await r.json();if(S.sq.trim()!==q)return;S.apiQ=q;S.apiHits=hs;S.apiSnip={};hs.forEach(h=>{if(SID[h.id])S.apiSnip[SID[h.id]]=h.snippet;});refreshHits();}catch(e){}},180);}
function snippet(d,q){const t=docText(d),i=t.toLowerCase().indexOf(q.toLowerCase());if(!q||i<0)return '';const a=Math.max(0,i-28);return (a?'… ':'')+esc(t.slice(a,i))+'<mark>'+esc(t.slice(i,i+q.length))+'</mark>'+esc(t.slice(i+q.length,i+q.length+46))+' …';}
function resultsHtml(){
  const h=hitsWithServer(),q=S.sq.trim();
  const allTags=[...new Set(Object.values(D).flatMap(d=>d.tags))].sort();
  if(h===null) return `<div class="chead first">Recent</div>${['radio link','tower','on-call'].map(x=>`<button class="hit" data-act="recent" data-id="${x}">${ic('search')}<span>${x}</span></button>`).join('')}
    <div class="chead">Tags</div><div class="tagrow">${allTags.map(t=>`<button class="tagc" data-act="ftag" data-tag="${t}">#${t}</button>`).join('')}</div>`;
  const answer=S.ai&&q&&h.length?`<div class="answer"><b>AI answer, made up for this mockup</b>A real answer would stand here in one or two sentences. Sources: ${h.slice(0,2).map(x=>D[x].n).join(', ')}.</div>`:'';
  const note=S.strat==='meaning'?'<p class="hint">In this mockup, this finds the same hits as full text.</p>':'';
  return `${answer}<div class="chead first">${h.length} ${h.length===1?'hit':'hits'}</div>${note}${h.map(x=>`<button class="hit" data-act="open" data-id="${x}">${ic(TYPE[D[x].t][0])}<span>${D[x].n}<small>${placeOf(x)}</small>${q?`<small class="snip">${((S.apiSnip&&S.apiQ===q&&S.apiSnip[x])||snippet(D[x],q))}</small>`:''}</span></button>`).join('')||'<p class="hint">Nothing found. Another strategy may help.</p>'}`;
}
function searchHtml(){
  const allTags=[...new Set(Object.values(D).flatMap(d=>d.tags))].sort(),n=S.ftags.length+(S.ftype?1:0);
  return `<div class="spanel"><button class="chip filterbtn" data-act="filters" aria-expanded="${S.filters}">${ic('view')}Filter${n?' ('+n+')':''}</button>
    ${S.filters?`<div class="filters"><select id="stype" aria-label="Type"><option value="">All types</option>${Object.keys(TYPE).filter(k=>!['dir','res','mail'].includes(k)).map(k=>`<option value="${k}"${S.ftype===k?' selected':''}>${TYPE[k][1]}</option>`).join('')}</select><div class="tagrow">${allTags.map(t=>`<button class="tagc" data-act="ftag" data-tag="${t}" aria-pressed="${S.ftags.includes(t)}">#${t}</button>`).join('')}</div></div>`:''}
    <div id="sres">${resultsHtml()}</div></div>`;
}
function sourceOf(d){
  if(d.blocks)return ['---',`title: ${d.n}`,'---',''].concat(...d.blocks.map(b=>[...b.raw.split('\n'),'']));
  const front=['---',`title: ${d.n}`,`width: ${d.w}`,`contents: ${d.toc}`,`tags: [${d.tags.join(', ')}]`,'---',''];
  const body=[d.doc.intro,''];
  d.doc.secs.forEach(s=>body.push('## '+s[0],'',plain(s[1]),''));
  return front.concat(body);
}
function statusHtml(d){
  let left;
  if(d.doc){const x=sourceOf(d),txt=plain(d.doc.intro)+' '+d.doc.secs.map(s=>s[0]+' '+plain(s[1])).join(' '),w=(txt.match(/\S+/g)||[]).length;
    left=`<span>${w} words</span><span>${txt.length} characters</span><span>${x.length} lines</span><span>${Math.max(1,Math.ceil(w/200))} min read</span>`;}
  else if(d.tab) left=`<span>${d.tab.rows.length} rows</span><span>${d.tab.head.length} columns</span>`;
  else if(d.t==='images') left='<span>6 images</span>';
  else if(d.html) left=`<span>${d.stat}</span>`;
  else left=`<span>${d.kids.length} children</span>`;
  return `<div class="status">${left}<span class="spacer"></span><span>${d.doc?'Markdown · UTF-8':TYPE[d.t][1]}</span></div>`;
}
function factsHtml(d){
  const u=d.uses||{},rows=(k,l)=>(u[k]||[]).map(x=>`<li><small>${l}</small><span>${x}</span></li>`).join('');
  const useRows=rows('ext','External')+rows('int','Internal')+rows('data','Data')+rows('vals','Value')+rows('db','Database');
  return `<div class="facts"><h3>Metadata</h3><dl><dt>Name</dt><dd>${d.n}</dd><dt>Type</dt><dd>${TYPE[d.t][1]}</dd>
    ${d.desc?`<dt>Description</dt><dd>${d.desc}</dd>`:''}<dt>In workspace</dt><dd>${placeOf(Object.keys(D).find(k=>D[k]===d))}</dd><dt>On disk</dt><dd>${d.path}</dd>
    ${d.by?`<dt>Created</dt><dd>${d.created||'Sep 12'} by ${d.cby||d.by}</dd><dt>Changed</dt><dd>${chg(d)}</dd>`:''}<dt>Visible to</dt><dd>${d.vis}</dd></dl>
    ${d.t!=='folder'&&!d.html?`<h3>Uses</h3>${useRows?`<ul>${useRows}</ul>`:'<div class="lab">Nothing yet.</div>'}
    <h3>Used in</h3>${(d.usedIn||[]).length?`<ul>${d.usedIn.map(x=>`<li><span>${x}</span></li>`).join('')}</ul>`:'<div class="lab">No other document.</div>'}
    <h3>Versions</h3>${d.blocks?versionsHtml(d):`<ul><li><small>now</small><span>${d.ch}</span></li><li><small>earlier</small><span>12 versions, with restore</span></li></ul>`}`:''}
    <h3>Later</h3><div class="lab">Export, save as (M-17) and share (M-5).</div></div>`;
}
/* Comments belong to an element of the page: a paragraph, heading, list or table, through its ID (ADR-18). */
function cmtHtml(t,i){
  const d=D[t.d],cs=d.comments||[],sel=t.cb,g={};cs.forEach(c=>(g[c.b]=g[c.b]||[]).push(c));
  const row=c=>`<div class="cmtrow"><b>${esc(c.who)}</b><em>${esc(c.when)}</em><p>${esc(c.t)}</p></div>`;
  const here=sel?`<div class="cfor">${ic('quote')}<span>${esc(t.cbl||sel)}</span><em>#${esc(sel)}</em></div>${(g[sel]||[]).map(row).join('')||'<p class="hint">No comments on this yet.</p>'}
    <input class="addtag" data-addcmt="${i}" placeholder="Write a comment, then Enter" aria-label="Write a comment">`
    :'<p class="hint">A comment belongs to an element of the page. Point at a paragraph, heading, list or table and click its speech bubble.</p>';
  const rest=Object.keys(g).filter(k=>k!==sel).map(k=>`<button class="cgo" data-act="cblk" data-w="${i}" data-id="${esc(k)}" data-l="${esc(g[k][0].l)}">${ic('quote')}<span>${esc(g[k][0].l)}</span><em>${g[k].length}</em></button>`).join('');
  return `<aside class="fdrawer" aria-label="Comments"><div class="fhead">${ic('chat')}Comments<button class="ic" data-act="cmt" data-w="${i}" aria-label="Close comments">${ic('x')}</button></div>
    ${here}${rest?`<h3>${sel?'Elsewhere on this page':'On this page'}</h3>${rest}`:''}</aside>`;
}
function gearHtml(t,i,hasSecs){
  const d=D[t.d],seg=(act,opts,cur)=>`<span class="seg" role="group">${opts.map(x=>`<button data-act="${act}" data-w="${i}" data-id="${x}" aria-pressed="${cur===x}">${x}</button>`).join('')}</span>`;
  return `<aside class="fdrawer" aria-label="Document settings"><div class="fhead">Document settings<button class="ic" data-act="gear" data-w="${i}" aria-label="Close settings">${ic('x')}</button></div>
    <h3>View</h3>
    ${hasSecs?`<div class="setrow"><span>Contents</span>${seg('settoc',['marker','side','hidden'],d.toc)}</div>`:''}
    <div class="setrow"><span>Width</span>${seg('setw',['slim','wide','full'],d.w)}</div>
    <div class="setrow"><span>Page view</span><button class="chip" disabled title="Comes later">DIN A4, later</button></div>
    <h3>Others can</h3><div class="setrow"><span>${d.acc?'Set here':'Inherited: '+ACC[accOf(t.d)].toLowerCase()}</span>${isOwner(t.d)?seg('setacc',['inherit','edit','comment','read'],d.acc||'inherit'):seg('setacc',['inherit','edit','comment','read'],d.acc||'inherit').replace(/<button /g,'<button disabled title="Only the owner sets this" ')}</div>
    <p class="hint">Both belong to this document, in its metadata. The source view shows them. Here a change lasts until you reload. In the product, saving from the web comes with editing (M-8).</p>
    <h3>Tags</h3><div class="tagrow">${d.tags.length?tagChips(d.tags,'tagsearch'):'<span class="lab">No tags yet.</span>'}</div>
    <input class="addtag" data-addtag="${i}" placeholder="Add a tag, then Enter" aria-label="Add a tag">
    ${factsHtml(d)}</aside>`;
}
function tabHtml(x,k,i){
  const on=k===S.wins[i].a;
  /* The general tab only shows while it holds a document. Older ones lie below it as a stack, and its arrow lists them. */
  if(x.general){if(!x.d)return '';const d=D[x.d],w=S.wins[i],older=olderOf(w).length;
    return `<div class="dtab general${on?' on':''}${older?' stack':''}" data-dw="${i}" data-dt="${k}" title="General tab: the next document replaces it. Drag it to keep it as a tab."><button class="sel" data-act="sel" data-w="${i}" data-t="${k}">${ic(TYPE[d.t][0])}<span>${d.n}</span></button>${older?`<button class="mini histbtn" data-act="hist" data-w="${i}" aria-expanded="${S.hist===i}" aria-label="Opened here before" title="Opened here before, up to 15">${ic('down')}</button>`:''}</div>`;}
  const d=D[x.d];
  return `<div class="dtab${on?' on':''}" data-dw="${i}" data-dt="${k}"><button class="sel" data-act="sel" data-w="${i}" data-t="${k}" title="${d.n}">${ic(TYPE[d.t][0])}<span>${d.n}</span></button>
    <button class="mini" data-act="close" data-w="${i}" data-t="${k}" title="Close" aria-label="Close ${d.n}">${ic('x')}</button></div>`;
}
/* Everything opens in the general tab. While a single window holds nothing else, neither window nor tab is drawn: the
   page stands alone, and a button beside New holds what the tab offers. A second tab or window brings the tabs back
   (the maintainer, 2026-11-09). */
const solo=()=>S.wins.length===1&&!S.wins[0].tabs.some(x=>!x.general);
function winHtml(w,i){
  const t=at(i),has=t&&t.d,cls=`win${i===S.focus?' focus':''}${solo()?' solo':''}`;
  const head=solo()?'':`<div class="tabrow"><div class="tabscroll">${w.tabs.map((x,k)=>tabHtml(x,k,i)).join('')}</div>
    <button class="ic tabend" data-act="tablist" data-w="${i}" aria-expanded="${S.tlist===i}" aria-label="All tabs of this window" title="All tabs of this window">${ic('list')}</button>${S.wins.length>1?`<button class="ic tabend" data-act="closewin" data-w="${i}" aria-label="Close window" title="Close window">${ic('x')}</button>`:''}${i===S.wins.length-1&&S.mods.ai?`<button class="ic tabend aibtn"${off('tool','aid')} data-act="aid" aria-pressed="${S.aid}" aria-label="AI assistant" title="AI assistant">${ic('ai')}</button>`:''}</div>`;
  const pop=(S.newAt===i?newMenuHtml('tab'):'')+(S.tmenu&&S.tmenu.w===i?`<div class="pop tabctx" role="menu" data-style="left:${S.tmenu.x}px">${tabMenuHtml(i,S.tmenu.t)}</div>`:'')
    +(S.hist===i?`<div class="pop histpop" role="menu"><b>Opened here before</b>${olderOf(w).map(x=>`<button data-act="open" data-id="${x}">${ic(TYPE[D[x].t][0])}${D[x].n}</button>`).join('')||'<span class="hint">Nothing yet.</span>'}</div>`:'')
    +(S.tlist===i?`<div class="pop tlistpop" role="menu"><b>Tabs of this window</b>${w.tabs.map((x,k)=>`<button data-act="sel" data-w="${i}" data-t="${k}" aria-pressed="${k===w.a}">${x.d?ic(TYPE[D[x.d].t][0])+D[x.d].n:'<em>empty general tab</em>'}</button>`).join('')}</div>`:'');
  if(!has) return `<section class="${cls}" data-win="${i}" aria-label="Window ${i+1}">${head}${pop}<div class="empty"><p>Empty.<br>What a list opens lands here, in the active window.</p></div></section>`;
  const d=D[t.d],{h,hs}=bodyHtml(t,i),mode=d.toc;
  const links=hs.map((x,j)=>`<button data-act="jump" data-w="${i}" data-id="w${i}h${j}">${x}</button>`).join('');
  const marker=hs.length&&mode==='marker'&&!t.src?`<span class="marker"><button class="chip" data-act="fly" data-w="${i}" aria-expanded="${t.fly}" aria-pressed="${t.fly}">${ic('list')}${hs.length} sections ▾</button>${t.fly?`<div class="fly"><b>In this document</b>${links}</div>`:''}</span>`:'';
  const find=t.find?`<span class="findin"><input data-findq="${i}" type="search" value="${esc(t.q)}" placeholder="Find on this page" aria-label="Find on this page"><span class="lab" data-findc="${i}"></span><button class="ic" data-act="findnav" data-w="${i}" data-id="-1" aria-label="Previous hit" title="Previous hit">${ic('up')}</button><button class="ic" data-act="findnav" data-w="${i}" data-id="1" aria-label="Next hit" title="Next hit">${ic('down')}</button></span>`:'';
  /* The heading is not content. The title sits below the breadcrumbs in the same bar, the tags to the right of the breadcrumbs. Created, changed and the rest are metadata. */
  const crumbs=placeOf(t.d),tg=d.tags.length?`<span class="tagline">${tagChips(d.tags,'tagsearch')}</span>`:'';
  const ac=accOf(t.d),badge=ac==='edit'?'':`<span class="accbadge" title="What others may do here">${ic(ac==='read'?'lock':'quote')}${ACC[ac]}</span>`;
  const place=`<span class="place" title="${crumbs}">${crumbs}</span>${tg}${badge}<span class="tspace"></span>`;
  const dhead=`<div class="dtitle"><h1>${d.n}</h1>${d.desc?`<p>${d.desc}</p>`:''}</div>`;
  const tools=`<div class="tools">${place}${marker}${find}
    <button class="ic" data-act="find" data-w="${i}" aria-pressed="${t.find}" aria-label="Find on this page" title="Find on this page">${ic('search')}</button>
    ${d.blocks&&d.canEdit?`<button class="ic" data-act="edit" data-w="${i}" aria-pressed="${!!t.edit}" aria-label="Edit" title="Edit block by block">${ic('pencil')}</button>`:''}
    ${d.doc?`<button class="ic" data-act="src" data-w="${i}" aria-pressed="${t.src}" aria-label="Source view" title="Source view">${ic('code')}</button>`:''}
    ${S.wins.length<4?`<button class="ic" data-act="split"${off('tool','split')} data-w="${i}" aria-label="Show also in a new column" title="Show also in a new column">${ic('split')}</button>`:''}
    ${accOf(t.d)!=='read'&&!d.tool?`<button class="ic" data-act="cmt" data-w="${i}" aria-pressed="${t.cmt}" aria-label="Comments" title="Comments${(d.comments||[]).length?', '+d.comments.length:''}">${ic('chat')}${(d.comments||[]).length?`<em class="cnum">${d.comments.length}</em>`:''}</button>`:''}
    <button class="ic" data-act="gear" data-w="${i}" aria-pressed="${t.gear}" aria-label="Document settings" title="Document settings">${ic('gear')}</button>
    <button class="ic" data-act="menu" data-w="${i}" aria-expanded="${S.menu===i}" aria-label="More" title="More">${ic('more')}</button>${S.menu===i?menuHtml(i):''}${dhead}</div>`;
  const inner=t.src?`<div class="src">${sourceOf(d).map(l=>`<div>${esc(l)}</div>`).join('')}</div>`:`<article class="doc ${d.w}">${h}</article>`;
  const toc=hs.length&&mode==='side'&&!t.src?`<nav class="toc"><b>Contents</b>${links}</nav>`:'';
  return `<section class="${cls}" data-win="${i}" aria-label="Window ${i+1}">${head}${pop}${tools}
    <div class="wbody">${toc}<div class="scroll" data-sc="${i}"><div data-art="${i}">${inner}</div></div>${t.gear?gearHtml(t,i,hs.length>0):''}${t.cmt&&accOf(t.d)!=='read'?cmtHtml(t,i):''}</div>${statusHtml(d)}</section>`;
}
function menuHtml(i){return `<div class="tabmenu" role="menu">${tabMenuHtml(i,S.wins[i].a)}</div>`;}
/* One menu for a tab, from a right click on it or from the dots: the same choices in both places. */
function tabMenuHtml(i,k){
  const w=S.wins[i],t=w.tabs[k],n=w.tabs.length,has=!!t.d,room=S.wins.length<4;
  const B=(act,label,extra='')=>`<button data-act="${act}" data-w="${i}" data-t="${k}" ${extra}>${mi(act)}${label}</button>`;
  return [has?B('close',t.general?'Clear':'Close','title="Also a middle click"'):'',n>1?B('closeothers','Close others'):'',k<n-1?B('closeright','Close to the right'):'',
    k>0?B('closeleft','Close to the left'):'',B('closeall','Close all'),
    has?'<hr>'+(t.general?B('keep','Keep as a tab of its own'):B('dup','Duplicate'))+B('copypath','Copy path'):'',
    has&&(room||S.wins.length>1)?'<hr>':'',has&&room?B('newwin','Move to new column, left',`data-id="${i}"${off('tool','newwin')}`)+B('newwin','Move to new column, right',`data-id="${i+1}"${off('tool','newwin')}`):'',
    has&&i>0?B('move','Move to the window on the left',`data-id="${i-1}"`):'',has&&i<S.wins.length-1?B('move','Move to the window on the right',`data-id="${i+1}"`):'',
    S.wins.length>1?'<hr>'+B('closewin','Close window'):''].join('');
}
function closeWhere(i,keep){
  const w=S.wins[i],cur=w.tabs[w.a];
  if(w.tabs[0]&&w.tabs[0].general&&!keep(0))Object.assign(w.tabs[0],newTab(null,true));
  w.tabs=w.tabs.filter((t,j)=>t.general||keep(j));
  const c=w.tabs.indexOf(cur);w.a=c>=0?c:Math.max(0,w.tabs.length-1);compact();
}
/* New always lands somewhere: in the chosen collection, or in Quick notes. The last choice stays. */
function newMenuHtml(where){
  const into=S.into,q=into==='qn';
  return `<div class="pop newpop ${where}" role="dialog" aria-label="New"><div class="intorow"><span>Into</span>${[['qn','Quick notes','note'],...C.filter(c=>!isDeco(c.id)).map(c=>[c.id,c.n,c.i])].map(x=>`<button class="chip" data-act="into" data-id="${x[0]}" aria-pressed="${into===x[0]}"${x[0]==='qn'?off('tool','qn'):''}>${ic(x[2])}${x[1]}</button>`).join('')}</div>
    <input data-quick placeholder="${q?'Just type, Enter keeps it as a note':'Just type, Enter makes a page'}" aria-label="New">
    <p class="hint">${q?'A card in Quick notes. It can move to a collection later.':'A Markdown page at the top of '+C.find(c=>c.id===into).n+'. Drag it to its place.'}</p>
    ${q?'<p class="hint">Pick a collection for other types.</p>':`<b>Or start as</b><div class="newtypes">${addTypes().map(k=>`<button data-act="newdoc" data-id="${k}"${off('add',k)}>${ic(TYPE[k][0])}${k==='page'?'Page, Markdown':TYPE[k][1]}</button>`).join('')}</div>`}</div>`;
}
function createIn(cid,k,text){
  const id='new'+(++S.newN),first=(text||'').split('\n')[0].trim();
  D[id]={t:k,n:first?first.slice(0,48):'New '+TYPE[k][1].toLowerCase(),w:'slim',toc:'marker',tags:[],kids:[],path:'not saved, this is a mockup',by:'you',ch:'now',vis:'As its collection',...none,stat:'new',
    html:text?`<p>${esc(text)}</p>`:'<p class="lab">Empty. Drag it in the list to where it belongs.</p>'};
  C.find(c=>c.id===cid).top.unshift(id);S.open[cid]=true;rebuildPlaces();S.just=id;return id;
}
/* Files and Resources are contexts of the left list. What they open lands in the same windows as everything else. */
TYPE.dir=['folder','Folder on disk'];TYPE.res=['db','Resource'];
const FILEROWS={'~':[['scans/','folder','–','Oct 12',''],['notes-2026.md','page','2 KB','today',''],['travel-expenses.xlsx','excel','40 KB','Oct 30','']],
  '~/scans':[['receipt-oct.pdf','page','220 KB','Oct 12','']],
  '':[['operations/','folder','–','Oct 31','Operations'],['projects/','folder','–','today','Projects']],
  operations:[['network/','folder','–','Oct 28','Network'],['on-call.md','page','4 KB','Oct 29','On-call handbook'],['site-logs/','folder','–','Oct 31','Site logs']],
  'operations/network':[['plan.md','page','6 KB','today','Network plan'],['plan/','folder','–','Oct 28','Network plan (children)'],['rack-photo.jpg','images','2.1 MB','Sep 11',''],['walkthrough.mp4','video','48 MB','Oct 12','Site walkthrough']],
  'operations/network/plan':[['vlans.md','page','1 KB','Oct 28','VLAN list']],
  projects:[['tower.md','page','3 KB','Oct 31','Tower 50 m'],['tower/','folder','–','today','Tower 50 m (children)'],['old-offer.pdf','page','410 KB','Mar 04','']],
  'projects/tower':[['tower-dimensions.xlsx','excel','88 KB','today','Tower dimensions'],['photos/','images','34 MB','Oct 18','Site photos'],['kickoff.pptx','slides','6 MB','Sep 30','Kickoff slides'],['~$tower-dimensions.xlsx','excel','hidden','today','hidden: lock file']]};
const FILEDOC={'on-call.md':'hand','plan.md':'plan','tower.md':'tower','tower-dimensions.xlsx':'dims','photos/':'photos','site-logs/':'logs','vlans.md':'vlans','walkthrough.mp4':'walk','kickoff.pptx':'deck'};
const FILETAGS={'network/':'network','on-call.md':'on-call','plan.md':'network','rack-photo.jpg':'hardware','tower.md':'tower','tower-dimensions.xlsx':'tower','photos/':'tower'};
const subOf=(dir,x)=>(dir?dir+'/':'')+x[0].replace(/\/$/,'');
const isDir=(dir,x)=>x[0].endsWith('/')&&!!FILEROWS[subOf(dir,x)];
const fileTarget=(dir,x)=>isDir(dir,x)?'dir:'+subOf(dir,x):FILEDOC[x[0]]||'file:'+subOf(dir,x);
const pseudo=(id,o)=>{D[id]={w:'wide',toc:'marker',tags:[],kids:[],uses:{},usedIn:[],...o};};
const tableHtml=(h,rows)=>`<div class="tw"><table><tr>${h.map(x=>`<th>${x}</th>`).join('')}</tr>${rows.map(r=>`<tr${r.dim?' data-style="opacity:.55"':''}>${r.map(x=>`<td>${x}</td>`).join('')}</tr>`).join('')}</table></div>`;
const openBtn=(id,label,icon)=>`<button class="l" data-act="open" data-id="${id}">${icon?ic(icon)+' ':''}${label}</button>`;
Object.keys(FILEROWS).forEach(dir=>{
  const rows=FILEROWS[dir].map(x=>{const hid=x[2]==='hidden',r=[hid?x[0]:openBtn(fileTarget(dir,x),x[0]),x[2],x[3],x[4]?x[4]:'<span class="unplaced">△ not placed</span>',FILETAGS[x[0]]?'#'+FILETAGS[x[0]]:''];r.dim=hid;return r;});
  pseudo('dir:'+dir,{t:'dir',n:dir==='~'?'My files':dir===''?'Share':dir.split('/').pop()+'/',path:dir==='~'?'your own folder':dir===''?'the share':dir+'/',place:'Files › '+(dir.startsWith('~')?dir.replace('~','My files'):'Share'+(dir?' › '+dir:'')),vis:dir.startsWith('~')?'Only you':'Follows the disk or share',stat:`${FILEROWS[dir].length} entries`,
    html:`<div class="note">Moving something here does not move it in your workspace, and the reverse holds too. Lock files stay hidden in normal use. Upload and share access come with M-2.</div>`+tableHtml(['Name','Size','Changed','In your workspace','Tags'],rows)});
  FILEROWS[dir].forEach(x=>{if(x[4]||x[2]==='hidden'||isDir(dir,x))return;pseudo('file:'+subOf(dir,x),{t:'dir',n:x[0],path:subOf(dir,x),place:'Files › '+dir,vis:'Follows the disk or share',stat:x[2],tags:FILETAGS[x[0]]?[FILETAGS[x[0]]]:[],html:'<div class="note">This file lies on the disk but has no place in your workspace yet.</div>'});});
});
TYPE.mail=['mail','Mail'];
const MAILS=[{id:5,box:'sent',from:'To J. Roth',date:'Oct 27',unread:false,subj:'Antenna swap',body:'Done, noted in the handbook.'},{id:6,box:'archive',from:'Supplier desk',date:'Sep 02',unread:false,subj:'Old offer',body:'Thanks for your interest.'},{id:1,box:'inbox',from:'J. Roth',date:'09:12',unread:true,subj:'Crane on Tuesday',body:'The crane company confirmed Tuesday at seven. Can you check the tower dimensions before then?'},
 {id:2,box:'inbox',from:'Supplier desk',date:'Mon',unread:true,subj:'Your switch license',body:'The license for your core switches runs out at the end of the month.'},
 {id:3,box:'inbox',from:'M. Sander',date:'Oct 30',unread:false,subj:'On-call handbook',body:'Please look at the new escalation levels in the handbook.'},
 {id:4,box:'inbox',from:'A. Keller',date:'Oct 28',unread:false,subj:'Rack photo',body:'New photo is on the share, but not placed yet.'}];
MAILS.forEach(m=>pseudo('mail:'+m.id,{t:'mail',n:m.subj,w:'slim',path:'not a file, from the mail account',place:'Mail › '+m.box,vis:'Only you',stat:'1 message',
  html:`<dl class="mailhead"><dt>From</dt><dd>${m.from}</dd><dt>To</dt><dd>you</dd><dt>Date</dt><dd>${m.date}</dd></dl><p>${m.body}</p><div class="note">Reply, and linking a mail to a document, are to be settled in M-22.</div>`}));
function diskRow(dir,x){
  const id=fileTarget(dir,x),folder=isDir(dir,x),sub=subOf(dir,x),t=at(S.focus),cur=t&&t.d===id,open=kidsOpen(id);
  const kids=folder?FILEROWS[sub].filter(y=>y[2]!=='hidden'):[];
  return `<li><div class="row${cur?' cur':''}"><button class="arrow ${folder?(open?'open':''):'none'}" data-act="tog" data-id="${id}" aria-label="Toggle ${x[0]}">${ic('chev')}</button>
    <button class="name" data-act="open" data-id="${id}" title="${x[0]}">${ic(TYPE[x[1]][0])}<span>${x[0]}</span></button>${!x[4]&&dir?'<span class="count" title="Not placed in the workspace">△</span>':''}</div>
    ${folder&&open?`<ul class="t">${kids.map(y=>diskRow(sub,y)).join('')}</ul>`:''}</li>`;
}
const RES=[
 {id:'values',n:'Values',m:'M-3',i:'list',head:['Name','Value','Source','Used in'],rows:[['tower.height','50 m','tower-dimensions.xlsx','4 documents'],['router.ip','192.0.2.1','_values.yaml','2 documents']]},
 {id:'snippets',n:'Snippets',m:'M-3',i:'code',head:['Name','Kind','Used in'],rows:[['customers','SQL','3 documents'],['maintenance-note','Text','7 documents']]},
 {id:'dbs',n:'Databases and views',m:'M-4',i:'db',head:['Name','Type','Placed in workspace','Used in','Tags'],rows:[['Inventory','Database','Projects','1 view','#hardware'],['Open tickets','Database view','Operations › Network','1 document','#network'],['Weekly export','Database view','△ not placed','0 documents','']]},
 {id:'ext',n:'External sources',m:'M-4',i:'box',head:['Name','Kind','Data age','State'],rows:[['Ticket system','REST','4 min','✓ working'],['Supplier list','CSV','2 d','△ last good state']]}];
const RESDOC={Inventory:'inv','Open tickets':'tickets'};
const resTarget=(g,r)=>RESDOC[r[0]]||`res:${g.id}/${r[0]}`;
RES.forEach(g=>{
  pseudo('res:'+g.id,{t:'res',n:g.n,path:'not a file',place:'Resources',vis:'Everyone with access to this workspace',stat:`${g.rows.length} entries`,
    html:`<div class="note">What has no place in a folder. Comes with ${g.m}.</div>`+tableHtml(g.head,g.rows.map(r=>[openBtn(resTarget(g,r),r[0]),...r.slice(1)]))});
  g.rows.forEach(r=>{if(!RESDOC[r[0]])pseudo(`res:${g.id}/${r[0]}`,{t:'res',n:r[0],path:'not a file',place:'Resources › '+g.n,vis:'Everyone with access to this workspace',stat:g.n,html:tableHtml(['Field','Content'],g.head.map((h,k)=>[h,r[k]]))});});
});
/* A window goes when only its empty general tab is left, unless it is the last window. */
function compact(){
  const f=S.wins[S.focus],gone=w=>w.tabs.length<=1&&!(w.tabs[0]&&w.tabs[0].d);
  S.wins=S.wins.filter(w=>!gone(w));if(!S.wins.length)S.wins=[{tabs:[newTab(null,true)],a:0,hist:[]}];
  S.wins.forEach(w=>{if(w.a>=w.tabs.length)w.a=w.tabs.length-1;if(w.a===0&&!w.tabs[0].d&&w.tabs.length>1)w.a=1;});
  S.focus=Math.max(0,S.wins.indexOf(f));
}
const MODS=[['mail','Mail','An entry in nav1'],['private','Private area','Entries at the bottom of nav1'],['bookmarks','Bookmarks','A document type, with an import'],['news','News','An entry in nav1'],['ai','AI assistant','A drawer on the right'],['network','Network','IP ranges and device lists, as document types'],['lists','To-do and Kanban','Two document types'],['video','Video','A document type'],['slides','PowerPoint','A document type'],['notes','Quick notes','A document type']];
const MODTYPE={bookmark:'bookmarks',todo:'lists',kanban:'lists',video:'video',slides:'slides',notes:'notes',ipnet:'network',devices:'network'};
const shown=id=>{const m=MODTYPE[D[id].t];return !m||S.mods[m];};
const STAGES=[50,84,116,172],STAGENAMES=[['0','Icon'],['1','Stacked'],['2','Text'],['3','Beside']];
const seg3=(act,opts,cur)=>`<span class="seg" role="group">${opts.map(x=>`<button data-act="${act}" data-id="${x[0]}" aria-pressed="${String(cur)===x[0]}">${x[1]}</button>`).join('')}</span>`;
function settingsHtml(sec){
  let b='';
  if(sec==='look') b=`<div class="shead">Appearance</div><div class="setrow"><span>Theme</span>${seg3('theme',[['system','System'],['light','Light'],['dark','Dark']],S.theme)}</div>
    <div class="setrow"><span class="withic">${ic('search')}Page scale</span><span class="stepper"><button class="ic" data-act="scale" data-id="-10" aria-label="Smaller">−</button><button class="chip" data-act="scale" data-id="0" title="Back to 100%">${S.scale}%</button><button class="ic" data-act="scale" data-id="10" aria-label="Larger">+</button></span></div>`;
  if(sec==='navi') b=`<div class="shead">Navigation</div>
    <div class="slab">Clicks</div><p class="hint">A click opens only this. Ctrl and click selects it too, Shift and click a range: dragging one moves them all, and on a window they open as tabs. A double click keeps a collection open, or no longer; a middle click opens a document as a tab of its own. A click on a collection closes the others unless they are kept open. Anything with children also opens them in nav3, and a second click closes them.</p>
    <div class="slab">Lists</div><label class="setrow" for="limit"><span>Open by themselves up to</span><span><input id="limit" type="number" min="0" max="50" value="${S.limit}"> children</span></label>
    <div class="slab">Width, or drag the right edge of each column</div>
    <div class="setrow"><span>Nav1</span>${seg3('s1',STAGENAMES,S.s1)}</div><div class="setrow"><span>Nav2</span>${seg3('s2',STAGENAMES,S.s2)}</div>
    <div class="setrow"><span>Nav3</span><button class="chip" data-act="w3reset" title="Back to the default">${S.w3} px, reset</button></div>`;
  if(sec==='lang') b='<div class="shead">Language</div><p class="hint">English for now. German comes as a translation of the interface.</p>';
  if(sec==='notif') b='<div class="shead">Notifications</div><p class="hint">Mail for mentions and replies comes with M-16.</p>';
  return `<div class="spanel">${b}<p class="hint">These belong to you. With the database your account keeps them; without, this browser does.</p></div>`;
}
const ADMINTXT={users:['Users and groups','Who exists, who is in which group, who may administer. Comes with M-5.'],src:['Sources','Connections to databases and other systems. Comes with M-4.'],store:['Storage and backup','Where documents lie, the share, the export. Comes with M-2.'],signin:['Sign-in','How people sign in. The gate in front decides today.'],audit:['Audit log','Who did what. Comes with M-2 and M-5.']};
function adminHtml(sec){
  if(sec!=='mods') return `<div class="spanel"><div class="shead">${ADMINTXT[sec][0]}</div><p class="hint">${ADMINTXT[sec][1]}</p></div>`;
  return `<div class="spanel"><div class="shead">Modules</div>${MODS.map(m=>`<button class="modrow" data-act="mod" data-id="${m[0]}" aria-pressed="${S.mods[m[0]]}"><span>${m[1]}<small>${m[2]}</small></span><em>${S.mods[m[0]]?'✓ On':'Off'}</em></button>`).join('')}<p class="hint">Off means gone for everyone, and no data is touched. Only administrators see this part of the profile.</p></div>`;
}
const listRow=(id,icon,label,side='')=>{const t=at(S.focus);return `<li><div class="row${t&&t.d===id?' cur':''}"><span class="arrow none"></span><button class="name" data-act="open" data-id="${id}" title="${label}">${ic(icon)}<span>${label}</span></button>${side?`<span class="count">${side}</span>`:''}</div></li>`;};
const BM=[{id:'bar',n:'Toolbar',i:'star',items:[{n:'Ticket system',u:'https://tickets.example.com',d:'Open tickets of the team.'},{n:'Supplier portal',u:'https://supplier.example.net',d:'Orders and licenses.'},{n:'Weather radar',u:'https://weather.example.org',d:'Before crane work.'}]},
  {id:'work',n:'Work',i:'folder',items:[{n:'Switch manual',u:'https://docs.example.com/switch',d:'Firmware 9, chapter on VLANs.'},{n:'Crane company',u:'https://crane.example.net',d:'Booking and phone numbers.'}]},
  {id:'read',n:'Reading list',i:'list',items:[{n:'Antenna mounting guide',u:'https://example.org/antenna',d:'Read before the mounting.'}]}];
const FEEDS=[{id:'all',n:'All',i:'list'},{id:'blog',n:'Project blog',i:'globe'},{id:'sec',n:'Security notes',i:'shield'},{id:'rel',n:'Release notes',i:'flag'}];
const NEWS=[{id:1,f:'sec',date:'09:40',unread:true,h:'Switch firmware 9.4 fixes a login flaw',b:'The vendor advises the update for all core switches. A made-up entry.'},
  {id:2,f:'rel',date:'Mon',unread:true,h:'Office suite 26.2 is out',b:'Better import of slides. A made-up entry.'},
  {id:3,f:'blog',date:'Oct 30',unread:false,h:'How we document a tower build',b:'A short story about pages, values and photos. A made-up entry.'}];
/* Private: only its user sees it. Each entry is a view like any other: nav1 opens its lists in nav2, nav2 opens one in nav3, nav3 opens into the active window. */
const PRIVV=[['todo','To-do','check'],['cal','Calendar','cal'],['time','Time','clock'],['vac','Vacation','sun'],['notes','Notes','note']];
const TL=[{id:'mine',n:'My to-dos',i:'check',items:[{t:'Renew the first aid course',done:false,due:'Nov 20'},{t:'Hand in travel expenses',done:true},{t:'Plan the winter holidays',done:false}]},
  {id:'shop',n:'Shopping',i:'list',items:[{t:'Cable ties',done:false},{t:'Labels',done:false},{t:'Gloves',done:true}]},
  {id:'car',n:'Car',i:'list',items:[{t:'Winter tyres',done:false,due:'Nov 14'},{t:'Inspection',done:false,due:'Dec 02'}]}];
const CALS=[{id:'me',n:'Personal',i:'cal',ev:{3:'Dentist',12:'Review',24:'Vacation',25:'Vacation'}},{id:'team',n:'Team',i:'user',ev:{7:'Crane call',18:'Team lunch'},src:'Shared by the project team'},{id:'hol',n:'Holidays',i:'globe',ev:{1:'Public holiday'},src:'Imported from a calendar file'}];
const BOARDS=[{id:'quick',n:'Quick notes',i:'note',doc:'qn'},{id:'ideas',n:'Ideas',i:'spark',doc:'nb:ideas'}];
const SORT={todo:TL,cal:CALS,notes:BOARDS,bookmarks:BM};
const PROFSET=[['acct','Account','user'],['look','Appearance','sun'],['navi','Navigation','panel'],['lang','Language','globe'],['notif','Notifications','mail']];
const PROFADM=[['mods','Modules','grid'],['users','Users and groups','user'],['src','Sources','db'],['store','Storage and backup','hd'],['signin','Sign-in','lock'],['audit','Audit log','list']];
const tickRow=(l,k,x)=>{const t=at(S.focus),id=`ti:${l.id}/${k}`;return `<li><div class="row${t&&t.d===id?' cur':''}"><button class="tick" data-act="tick" data-l="${l.id}" data-id="${k}" aria-pressed="${x.done}" aria-label="Done: ${esc(x.t)}">${x.done?ic('check'):'<span class="box"></span>'}</button><button class="name${x.done?' done':''}" data-act="open" data-id="${id}" title="${esc(x.t)}"><span>${esc(x.t)}</span></button>${x.due?`<span class="count">${x.due}</span>`:''}</div></li>`;};
/* Private lists follow the rule of collections: a click opens one and closes what is not kept open, a double click keeps one open. */
function privPart(v,sec){
  const pinned=(S.spin[v]||{})[sec],hd=(n,doc,add='')=>`<div class="cheadrow" data-sechead="${sec}"><button class="chead" data-act="open" data-id="${doc}" title="Open ${esc(n)}">${pinned?ic('pin'):''}${esc(n)}</button>${add}</div>`;
  if(v==='todo'){const l=TL.find(x=>x.id===sec);if(!l)return '';return hd(l.n,'tl:'+l.id,`<button class="addbtn" data-act="tadd" data-id="${l.id}" aria-label="Add a to-do" title="Add a to-do">${ic('plus')}</button>`)+`<ul class="t top">${l.items.map((x,k)=>tickRow(l,k,x)).join('')}</ul>`;}
  if(v==='cal'){const cs=sec==='all'?CALS:CALS.filter(c=>c.id===sec);if(!cs.length)return '';const ev=cs.flatMap(c=>Object.entries(c.ev).map(([d,n])=>[+d,n,c])).sort((a,b)=>a[0]-b[0]);
    return hd(sec==='all'?'All calendars':cs[0].n,'cal:'+sec)+`<ul class="t top">${ev.map(e=>listRow(`ev:${e[2].id}/${e[0]}`,'clock',e[1],'Nov '+e[0])).join('')}</ul>${sec!=='all'&&cs[0].src?`<p class="hint pad">${cs[0].src}.</p>`:''}`;}
  if(v==='time'){const P={week:['priv:time','This week',[['Mon','8:12'],['Tue','8:27'],['Wed','7:45'],['Thu','7:16']]],last:['time:last','Last week',[['Mon','8:00'],['Tue','8:05'],['Wed','8:30'],['Thu','7:50'],['Fri','6:10']]],oct:['time:oct','October',[['Week 40','39:10'],['Week 41','40:05'],['Week 42','38:20'],['Week 43','40:00']]]}[sec];if(!P)return '';
    return hd(P[1],P[0])+`<ul class="t top">${P[2].map(r=>listRow(P[0],'cal',r[0],r[1])).join('')}</ul>`;}
  if(v==='vac'){const P={y26:['priv:vac','2026',[['Mar 02 – 13','10 days ✓'],['Jul 20 – 27','8 days ✓'],['Nov 24 – 28','5 days △']]],y27:['vac:27','2027',[]]}[sec];if(!P)return '';
    return hd(P[1],P[0])+`<ul class="t top">${P[2].map(r=>listRow(P[0],'sun',r[0],r[1])).join('')}</ul>`;}
  if(v==='notes'){const b=BOARDS.find(x=>x.id===sec);if(!b)return '';const ns=D[b.doc].notes;return hd(b.n,b.doc)+`<ul class="t top">${ns.map(n=>listRow(b.doc,'note',esc(n.h||n.b.split('\n')[0]))).join('')}</ul>`;}
  return '';
}
function privNav3(v){
  const lock=`<div class="lockline">${ic('lock')}Only you can see this.</div>`,so=S.so[v]||{};
  const parts=SECTIONS[v]().filter(x=>so[x[0]]).map(x=>privPart(v,x[0])).join('');
  return `<div class="tree" id="tree">${lock}${parts||'<p class="lab pad">Nothing open. Pick a list in nav2.</p>'}</div>`;
}
function accountHtml(){
  const u=S.user;
  return `<div class="spanel"><div class="who"><span class="avatar">${esc(u.initials)}</span><span>${esc(u.name)}<small>${esc(u.email||u.login)}</small></span></div><div class="tagrow">${u.groups.map(g=>`<span class="tagc">${esc(g)}</span>`).join('')}</div>
    <p class="hint">Name, mail and groups come from the sign-in. Favorites, settings and the private area belong to this account.</p><a class="chip signout" href="/.tinysesam/logout">Sign out</a></div>`;
}
function nav3Html(){
  const v=S.view,sec=S.sec[v];
  if(v==='workspace') return `<div class="tree" id="tree">${treeHtml()}</div>`;
  if(v==='search') return searchHtml();
  if(v==='profile'){const sec=S.sec.profile;return sec==='acct'?accountHtml():PROFADM.some(x=>x[0]===sec)?adminHtml(sec):settingsHtml(sec);}
  if(PRIVV.some(x=>x[0]===v)) return privNav3(v,sec);
  if(v==='files'){
    if(sec==='ext') return `<div class="spanel"><div class="shead">Add a source</div>${[['SMB share','A Windows or NAS share'],['WebDAV','A cloud folder over WebDAV'],['SFTP','A folder on a server'],['S3','An object store bucket']].map(x=>`<button class="opt" disabled>${x[0]}<small>${x[1]}</small></button>`).join('')}<p class="hint">Later. A source you add shows here next to your files and the share, and every user sees only the sources they may use.</p></div>`;
    const root=sec==='home'?'~':'',head=sec==='home'?'My files · only you':'Share';
    return `<div class="tree" id="tree"><div class="chead first">${head}</div><ul class="t top">${listRow('dir:'+root,'folder',sec==='home'?'All my files':'All of the share')}${FILEROWS[root].map(x=>diskRow(root,x)).join('')}</ul></div>`;}
  if(v==='bookmarks'){
    if(sec==='import') return `<div class="tree" id="tree"><div class="chead first">Import</div><ul class="t top">${listRow('bm:import','down','From a browser')}</ul><p class="hint pad">Every browser exports its bookmarks as one HTML file.</p></div>`;
    const f=BM.find(x=>x.id===sec);return `<div class="tree" id="tree"><div class="chead first">${f.n}</div><ul class="t top">${f.items.map((b,k)=>listRow(`bm:${f.id}/${k}`,'globe',b.n)).join('')}</ul></div>`;}
  if(v==='news'){
    if(sec==='sub') return `<div class="tree" id="tree"><div class="chead first">Subscribe</div><ul class="t top">${listRow('news:sub','plus','Add a feed')}</ul></div>`;
    const t=at(S.focus),its=NEWS.filter(n=>sec==='all'||n.f===sec);return `<div class="tree" id="tree"><div class="chead first">${FEEDS.find(x=>x.id===sec).n}</div>${its.map(n=>{const id='news:'+n.id;return `<button class="mailrow${t&&t.d===id?' cur':''}${n.unread?' unread':''}" data-act="open" data-id="${id}"><span class="mfrom">${FEEDS.find(x=>x.id===n.f).n}<em>${n.date}</em></span><span class="msubj">${n.unread?'● ':''}${n.h}</span></button>`;}).join('')}<p class="hint">Later. Made-up entries.</p></div>`;}
  if(v==='resources'){const g=RES.find(x=>x.id===sec);return `<div class="tree" id="tree"><div class="chead first">${g.n}</div><ul class="t top">${listRow('res:'+g.id,'list','All '+g.n.toLowerCase())}${g.rows.map(r=>listRow(resTarget(g,r),g.i,r[0])).join('')}</ul></div>`;}
  if(v==='mail'){const t=at(S.focus),ms=MAILS.filter(m=>m.box===sec);return `<div class="tree" id="tree"><div class="chead first">${sec}</div>${ms.map(m=>{const id='mail:'+m.id;return `<button class="mailrow${t&&t.d===id?' cur':''}${m.unread?' unread':''}" data-act="open" data-id="${id}"><span class="mfrom">${m.from}<em>${m.date}</em></span><span class="msubj">${m.unread?'● ':''}${m.subj}</span></button>`;}).join('')||'<p class="hint">Nothing here.</p>'}<p class="hint">Comes with M-22. Made-up mail.</p></div>`;}
  return '';
}
function render(){
  if(S.booted)saveSettings();
  /* A block being written keeps the focus through a render; a new block asks for it with S.focusNext. */
  const ae=document.activeElement,ren=ae&&ae.dataset&&ae.dataset.ren!==undefined?{v:ae.value,a:ae.selectionStart,b:ae.selectionEnd}:null,want=S.focusNext||(ae&&ae.dataset&&ae.dataset.blk!==undefined?{d:ae.dataset.d,id:ae.dataset.blk,pos:ae.selectionStart}:null);S.focusNext=null;
  const root=document.documentElement,ph=phone();
  if(window.udSetTheme&&window.udTheme&&window.udTheme()!==S.theme)window.udSetTheme(S.theme);
  root.style.zoom=S.scale/100;
  root.style.setProperty('--w1',(ph?70:STAGES[S.s1])+'px');root.style.setProperty('--w2',(ph?78:STAGES[S.s2])+'px');root.style.setProperty('--w3',S.w3+'px');
  const help='';
  /* No bar of its own: name, New and search sit above the columns, the tabs of each window in the same row beside them. */
  const wide=S.rail&&!ph;
  const bar=`<div class="brandrow${wide?'':' tight'}"><span class="brand" title="UnderDoc"><img src="${LOGO}" alt=""><span>UnderDoc</span></span>
    ${wide?`<input class="search" type="search" id="search" value="${esc(S.sq)}" placeholder="Search /" aria-label="Search">`:''}
    <button class="ic newbtn" data-act="newmenu" aria-expanded="${S.newAt==='bar'}" aria-label="New" title="New: type a note or pick a type">${ic('plus')}</button>${solo()?`<button class="ic newbtn winbtn" data-act="winmenu" aria-expanded="${!!S.ctx&&S.ctx.kind==='win'}" aria-label="Window and tabs" title="Window and tabs: keep as a tab, a new column, opened before">${ic('win')}</button>`:''}${S.newAt==='bar'?newMenuHtml('bar'):''}</div>`;
  const n1=(v,n,i)=>navItem('view',v,n,i,S.view===v);
  const privs=S.mods.private?`<span class="sep"></span><span class="n1lab" title="Only you see this">${ic('lock')}<span class="lbl">Private</span></span>${PRIVV.map(x=>n1(x[0],x[1],x[2])).join('')}`:'';
  const strip=`<nav class="strip col s${ph?1:S.s1}" aria-label="Nav1"><div class="n1top">${n1('workspace','Workspace','panel')}${n1('files','Files','hd')}${n1('resources','Resources','db')}${S.mods.news?n1('news','News','globe'):''}${S.mods.mail?n1('mail','Mail','mail'):''}<span class="sep"></span>${n1('search','Search','search')}<span class="spacer"></span>${privs}</div><div class="n1bot"><span class="sep"></span>
    <button class="navitem" data-act="view" data-id="profile" aria-pressed="${S.view==='profile'}" title="Profile, settings and administration"><span class="avatar tiny">${esc(S.user.initials)}</span><span class="lbl">Profile</span></button></div><div class="grip" data-grip="1" role="separator" aria-orientation="vertical" aria-label="Width of nav1" tabindex="0" title="Drag: snaps to the next view"></div></nav>`;
  const prof='';
  const s2=ph?1:S.s2;
  const rail=S.rail?`<aside class="rail" aria-label="Navigation"><nav class="nav2 col s${s2}" aria-label="Nav2">${nav2Html()}<div class="grip" data-grip="2" role="separator" aria-orientation="vertical" aria-label="Width of nav2" tabindex="0" title="Drag: snaps to the next view"></div></nav><div class="nav3" aria-label="Nav3">${nav3Html()}<div class="grip" data-grip="3" role="separator" aria-orientation="vertical" aria-label="Width of nav3" tabindex="0" title="Drag to change the width"></div></div></aside>`:'';
  const name=w=>{const t=w.tabs[w.a];return t&&t.d?D[t.d].n:'Empty';};
  const winbar=S.wins.length>1?`<div class="winbar">${S.wins.map((w,i)=>`<button class="chip" data-act="focus" data-id="${i}" aria-pressed="${i===S.focus}">${name(w)}</button>`).join('')}</div>`:'';
  const work=`<div class="work">${winbar}<div class="wins${S.wins.length>1?' many':''}" data-style="grid-template-columns:repeat(${S.wins.length},minmax(0,1fr))">${S.wins.map(winHtml).join('')}</div></div>`;
  rendering=true;document.getElementById('root').innerHTML=help+`<div class="main"><div class="side">${bar}<div class="navs">${strip}${rail}</div></div>${work}${S.aid&&S.mods.ai?aiHtml():''}</div>${S.toast?`<div class="toast" role="status">${esc(S.toast)}</div>`:''}${ctxHtml()}${addPop()}`;
  S.wins.forEach((w,i)=>{const t=at(i),e=document.querySelector(`[data-sc="${i}"]`);if(t&&e)e.scrollTop=t.top;applyFind(i);});
  rendering=false;refocusBlock(want);
  /* A name being written keeps its text and the focus through a render. */
  const rn=ren&&document.querySelector('[data-ren]');if(rn){rn.value=ren.v;rn.focus();rn.setSelectionRange(ren.a,ren.b);}
  const tr=document.getElementById('tree');if(tr)tr.scrollTop=S.treeTop;
  /* A menu with no room below its heading opens above it. */
  /* A menu stays inside the window: at the edge it moves in, and a long one scrolls (2026-11-09). Its place lives in
     data-style, which is applied after a render, so a moved place is written there too. */
  const stay=(el,up)=>{if(!el)return;applyStyles(el);const z=S.scale/100,r=el.getBoundingClientRect(),cs=el.style;let x=parseFloat(cs.left),y=parseFloat(cs.top),moved=false;
    if(r.right>innerWidth-4&&!isNaN(x)){x=Math.max(0,x-(r.right-innerWidth+4)/z);moved=true;}
    if(r.bottom>innerHeight-4&&!isNaN(y)){y=up!==undefined?Math.max(4,up-r.height/z):Math.max(4,(innerHeight-4-r.height)/z);moved=true;}
    if(moved){el.dataset.style=(isNaN(x)?'':`left:${x}px;`)+(isNaN(y)?'':`top:${y}px`);el.style.cssText=el.dataset.style;}};
  stay(S.ctx&&document.querySelector('.ctxpop:not(.cmdpop)'));stay(document.querySelector('.tabctx'));
  /* The menu New opens below its button, or above it where there is no room, and the field takes the typing at once. */
  const ap=document.querySelector('.cmdpop');if(ap){stay(ap,S.addAt.up);filterNew(S.addQ);const f=ap.querySelector('[data-nfilter]');f.focus();f.setSelectionRange(f.value.length,f.value.length);}
  /* Every element of a page has its ID, and a speech bubble beside it holds its comments. */
  document.querySelectorAll('[data-art]').forEach(art=>{const i=+art.dataset.art,t=at(i);if(!t||!t.d||t.src||accOf(t.d)==='read'||D[t.d].tool)return;const doc=art.querySelector('article.doc');if(!doc)return;
    const cs=D[t.d].comments||[],n={p:0,h:0,t:0,l:0,c:0};
    doc.querySelectorAll('p,h2,h3,ul,ol,pre,.tw,.table-container').forEach(el=>{if(el.parentElement.closest('.embed,.kids,.tw,.table-container,ul,ol,.todos,.keep,.board,.facts,.note,dl')||el.matches('.lab,.hint,.note')||el.closest('.embed'))return;
      const k=el.matches('.tw, .table-container')?'t':el.matches('h2,h3')?'h':el.matches('ul,ol')?'l':el.matches('pre')?'c':'p',id=k+(++n[k]),cnt=cs.filter(c=>c.b===id).length;
      const lab=k==='t'?'Table: '+[...el.querySelectorAll('th')].map(x=>x.textContent).join(', '):el.textContent.trim().slice(0,60);
      if(t.cmt&&t.cb===id)el.classList.add('cfocus');
      const b=document.createElement('button');b.className='cblk'+(cnt?' has':'');b.dataset.act='cblk';b.dataset.w=i;b.dataset.id=id;b.dataset.l=lab;b.title=cnt?cnt+' comments on this':'Comment on this';b.setAttribute('aria-label',b.title);b.innerHTML=ic('chat')+(cnt?`<em>${cnt}</em>`:'');
      el.after(b);b.style.top=el.offsetTop+'px';});});
  /* A table made in a page gets its ID right away. Its address can be pasted into another page, where it shows live. */
  document.querySelectorAll('[data-art]').forEach(art=>{const t=at(+art.dataset.art);if(!t||!t.d||t.src||D[t.d].tool)return;
    art.querySelectorAll('article.doc table').forEach((tb,k)=>{if(tb.closest('.embed'))return;const own=tb.closest('[data-b]'),part=own?own.dataset.b:'t'+(k+1),b=document.createElement('button');b.className='blk';b.dataset.act='blocklink';b.dataset.id=t.d+'#'+part;b.title='Copy the address of this table';b.innerHTML=ic('opennew')+'<span>#'+part+'</span>';(tb.closest('.tw, .table-container')||tb).before(b);});});
}
/* AI drawer on the right: it knows the document in the active window. */
const aiAnswer=()=>{const t=at(S.focus);return t&&t.d?`Made up for this mockup. A real answer about “${D[t.d].n}” would stand here, with links to its sources.`:'Made up for this mockup. Open a document, and the answer can refer to it.';};
function aiHtml(){
  const t=at(S.focus),about=t&&t.d?D[t.d].n:'nothing open';
  return `<aside class="aidrawer" aria-label="AI assistant"><div class="fhead">${ic('ai')}AI assistant<button class="ic" data-act="aid" aria-label="Close AI assistant">${ic('x')}</button></div>
    <div class="aictx">About: <b>${esc(about)}</b></div>
    <div class="aichat">${S.chat.map(m=>`<div class="aiq">${esc(m.q)}</div><div class="aia">${m.a}</div>`).join('')||'<p class="hint">Ask about the open document, or let it draft something.</p>'}</div>
    <div class="tagrow">${['Summarize this','Find related documents','Draft a note from this'].map(x=>`<button class="tagc" data-act="aiask" data-id="${x}">${x}</button>`).join('')}</div>
    <input class="aiin" data-aiq placeholder="Ask, then Enter" aria-label="Ask the AI">
    <p class="hint">Which model, and where it runs, is settled in M-13. It is off until an administrator sets it up.</p></aside>`;
}
function applyFind(i,scroll){
  const t=at(i);if(!t||!t.find)return;
  const root=document.querySelector(`[data-art="${i}"]`),out=document.querySelector(`[data-findc="${i}"]`);
  if(!root||!out)return;
  if(!t.q){out.textContent='';return;}
  const re=new RegExp(t.q.replace(/[.*+?^${}()|[\]\\]/g,'\\$&'),'gi'),nodes=[],tw=document.createTreeWalker(root,NodeFilter.SHOW_TEXT);
  while(tw.nextNode())nodes.push(tw.currentNode);
  nodes.forEach(n=>{const s=n.nodeValue;re.lastIndex=0;if(!re.test(s))return;re.lastIndex=0;const f=document.createDocumentFragment();let last=0,m;
    while((m=re.exec(s))){f.append(s.slice(last,m.index));const k=document.createElement('mark');k.textContent=m[0];f.append(k);last=m.index+m[0].length;}
    f.append(s.slice(last));n.replaceWith(f);});
  const ms=[...root.querySelectorAll('mark')];
  if(!ms.length){out.textContent='no hit';return;}
  t.cur=((t.cur%ms.length)+ms.length)%ms.length;
  ms[t.cur].classList.add('cur');out.textContent=`${t.cur+1} of ${ms.length}`;
  if(scroll)ms[t.cur].scrollIntoView({block:'center'});
}
/* Every window has its own general tab, and a list opens into the active window. */
function openDoc(id){
  const w=S.wins[S.focus]||S.wins[0];let k=w.tabs.findIndex(t=>t.d===id);
  if(k<0){k=0;w.tabs[0]=newTab(id,true);w.hist=[id,...(w.hist||[]).filter(x=>x!==id)].slice(0,15);}
  w.a=k;if(id.startsWith('mail:'))MAILS.find(m=>'mail:'+m.id===id).unread=false;if(phone())S.rail=false;
  if(D[id]&&D[id].real){if(!D[id].loaded)loadDoc(id);const a=addressOf(id);if(location.pathname!==a)history.pushState({id},'',a);}
}
/* What was opened here before and is not open as a tab of its own. */
const olderOf=w=>(w.hist||[]).filter(x=>D[x]&&x!==w.tabs[0].d&&!w.tabs.some((t,j)=>j>0&&t.d===x));
/* When the general tab gives up its document, the one opened before takes its place. */
function fallback(w){const g=w.tabs[0],n=olderOf(w)[0]||null;Object.assign(g,newTab(n,true));if(!n&&w.a===0&&w.tabs.length>1)w.a=1;}
function popGeneral(w){const g=w.tabs[0];w.hist=(w.hist||[]).filter(x=>x!==g.d);fallback(w);}
function take(w,k){const t=w.tabs[k];
  if(t.general){const c={...t,general:false,gear:false,fly:false};fallback(w);return c;}
  w.tabs.splice(k,1);if(w.a>k)w.a--;return t;}
function placeTab(i,k,to,p){
  const src=S.wins[i],dst=S.wins[to],t=take(src,k);
  p=Math.max(1,Math.min(p,dst.tabs.length));
  dst.tabs.splice(p,0,t);dst.a=p;S.focus=to;compact();
}
/* Dragging the general tab keeps its document as a tab where it lands, and the general tab stays, empty. */
function newColumn(i,k,pos){
  if(S.wins.length>=4)return;
  const t=take(S.wins[i],k),f={tabs:[newTab(null,true),t],a:1,hist:[]};
  S.wins.splice(pos,0,f);S.focus=pos;compact();S.focus=Math.max(0,S.wins.indexOf(f));
}
/* One rule everywhere: a click opens only this, a double click keeps it open, Ctrl and click selects. */
function onlyThis(id){const o={};C.forEach(c=>{if(S.pin[c.id])o[c.id]=true;});o[id]=true;S.open=o;}
function togglePin(id){if(S.pin[id]){S.pin[id]=false;S.open[id]=false;}else{S.pin[id]=true;S.open[id]=true;}}
/* The second click of a double click comes with a click count of 2. The list is drawn anew after the first click, so the browser's dblclick event may never arrive. */
/* New opens below its row: the head of nav2 or of the collection. */
function openNew(id,anchor){const z=S.scale/100,r=anchor?anchor.getBoundingClientRect():{left:0,top:0,bottom:0};S.addIn=id;S.addQ='';S.addAt={x:Math.round(r.left/z),y:Math.round(r.bottom/z)+2,up:Math.round(r.top/z)-2};}
function collClick(id,e){if(e.detail===2)togglePin(id);else if(e.detail<2)onlyThis(id);}
/* Ctrl and click adds an entry of nav2 or nav3 to a selection or takes it out, Shift and click takes the range from the
   last one; a selection stays in one list. Keeping a collection open is a double click or the right click. */
const SELECTABLE='.nav3 .row[data-row],#clist .collrow[data-favid],.nav2 [data-secid]';
const zoneOfEl=n=>n.closest('.nav3')?'n3':n.matches('[data-secid]')?'sec':'n2';
const idOfEl=n=>n.dataset.row||n.dataset.favid||n.dataset.secid;
const isSel=(z,id)=>!!S.sel&&!S.sel.soft&&S.sel.z===z&&S.sel.ids.includes(id);
/* The entry clicked last, or the document open in the window, counts as selected without showing it, so Ctrl and Shift
   add to it. */
function selectClick(el,e){
  const z=zoneOfEl(el),id=idOfEl(el);
  if(!S.sel||S.sel.z!==z){const t=at(S.focus),cur=z==='n3'&&t&&t.d&&elOf('row',t.d)?t.d:null;S.sel={z,ids:cur?[cur]:[],anchor:cur};}
  S.sel.soft=false;
  const all=[...document.querySelectorAll(SELECTABLE)].filter(n=>zoneOfEl(n)===z).map(idOfEl);
  if(e.shiftKey&&S.sel.anchor&&all.includes(S.sel.anchor)){const a=all.indexOf(S.sel.anchor),b=all.indexOf(id);S.sel.ids=all.slice(Math.min(a,b),Math.max(a,b)+1);}
  else{const k=S.sel.ids.indexOf(id);if(k<0)S.sel.ids.push(id);else S.sel.ids.splice(k,1);S.sel.anchor=id;}
  if(!S.sel.ids.length)S.sel=null;
}
function openTab(id){const w0=S.wins[S.focus];let k=w0.tabs.findIndex((x,j)=>j>0&&x.d===id);if(k<0){w0.tabs.push(newTab(id));k=w0.tabs.length-1;}w0.a=k;}
document.addEventListener('scroll',e=>{const x=e.target;if(!x.dataset)return;if(x.dataset.sc!==undefined){const t=at(+x.dataset.sc);if(t)t.top=x.scrollTop;}else if(x.id==='tree')S.treeTop=x.scrollTop;},true);
document.addEventListener('click',e=>{
  if(noClick){noClick=false;e.stopPropagation();e.preventDefault();return;}
  if(e.target.closest('input,select,label'))return;
  const pick=(e.ctrlKey||e.metaKey||e.shiftKey)&&!e.target.closest('.arrow,.pop')&&e.target.closest(SELECTABLE);
  if(pick){e.preventDefault();selectClick(pick,e);S.ctx=null;render();return;}
  if(S.sel&&!e.target.closest('.ctxpop'))S.sel=null;
  /* A plain click on an entry is where a range starts: Shift and click then takes everything up to there. */
  const plain=!e.target.closest('.arrow,.pop')&&e.target.closest(SELECTABLE);
  if(plain)S.sel={z:zoneOfEl(plain),ids:[idOfEl(plain)],anchor:idOfEl(plain),soft:true};
  const b=e.target.closest('button[data-act]'),win=e.target.closest('[data-win]'),act=b&&b.dataset.act;
  if(e.target.closest('.pop')&&!b)return;
  let dirty=S.menu!==null||S.tmenu||S.hist!==null||S.tlist!==null||S.addIn!==null||S.ctx!==null||S.newAt!==null;
  const hadMenu=S.menu,hadT=S.tmenu,hadHist=S.hist,hadTl=S.tlist,hadAdd=S.addIn,hadNew=S.newAt,hadCtx=S.ctx&&S.ctx.kind,ctx0=S.ctx,hadAsk=S.trashAsk;
  if(hadAsk){S.trashAsk=null;dirty=true;}
  if(win&&+win.dataset.win!==S.focus){S.focus=+win.dataset.win;dirty=true;}
  S.menu=null;S.tmenu=null;S.hist=null;S.tlist=null;S.addIn=null;S.ctx=null;S.newAt=null;
  if(!b){if(dirty)render();return;}
  const a=b.dataset,i=a.w!==undefined?+a.w:S.focus,w=S.wins[i],t=at(i),tk=a.t!==undefined?+a.t:w&&w.a;
  if(act!=='add')S.just=null;
  switch(act){
    case 'help':S.help=!S.help;break;
    case 'first':S.first=!S.first;if(S.first){if(S.into==='qn')S.into=C[0].id;if(LATER.view.includes(S.view))S.view='workspace';S.aid=false;}break;
    case 'rail':S.rail=!S.rail;break;
    case 'view':if(S.view===a.id&&S.rail&&a.id!=='search'){S.rail=false;break;}if(a.id==='search'&&S.view!=='search')S.prev=S.view;if(S.view!==a.id)S.treeTop=0;S.view=a.id;S.rail=true;if(a.id==='search'){render();const f=document.getElementById('search');f.focus();f.setSelectionRange(f.value.length,f.value.length);return;}break;
    case 'sec':{const v=S.view;S.treeTop=0;
      /* Where a third column adds nothing, nav2 opens straight into the window. */
      const ad={cal:{import:'cal:import'},time:{export:'time:export'},vac:{plan:'vac:new'},bookmarks:{import:'bm:import'},news:{sub:'news:sub'}}[v];if(ad&&ad[a.id]){openDoc(ad[a.id]);break;}
      if(v==='todo'&&a.id==='newlist'){const id='l'+(++S.newN);TL.push({id,n:'New list',i:'list',items:[]});todoDocs();S.sec.todo=id;S.so.todo[id]=true;break;}
      if(v==='notes'&&a.id==='newboard'){const id='b'+(++S.newN);D['nb:'+id]=boardDoc('New board',[]);BOARDS.push({id,n:'New board',i:'note',doc:'nb:'+id});S.sec.notes=id;S.so.notes[id]=true;break;}
      /* Private lists open like collections: a click opens one and closes what is not kept open, a double click keeps it open or stops. */
      if(S.so[v]){const k=a.id,sp=S.spin[v]=S.spin[v]||{};
        if(e.detail===2){if(sp[k]){sp[k]=false;S.so[v][k]=false;}else{sp[k]=true;S.so[v][k]=true;}}
        else if(e.detail<2){const o={};for(const x in sp)if(sp[x])o[x]=true;o[k]=true;S.so[v]=o;}}
      S.sec[v]=a.id;break;}
    case 'tick':{const it=TL.find(l=>l.id===a.l).items[+a.id];it.done=!it.done;break;}
    case 'tadd':{const l=TL.find(x=>x.id===a.id);l.items.push({t:'New to-do',done:false});todoDocs();openDoc(`ti:${l.id}/${l.items.length-1}`);break;}
    case 'priv':openDoc(a.id);break;
    case 'newmenu':{if(hadNew===(a.w!==undefined?i:'bar'))break;S.newAt=a.w!==undefined?i:'bar';if(a.w!==undefined)S.focus=i;render();const f=document.querySelector('[data-quick]');if(f)f.focus();return;}
    case 'newdoc':openDoc(createIn(S.into,a.id,''));break;
    case 'into':S.into=a.id;S.newAt=hadNew;render();{const f=document.querySelector('[data-quick]');if(f)f.focus();}return;
    case 'addmenu2':{S.view='workspace';S.rail=true;S.open[a.id]=true;render();openNew(a.id,document.querySelector(`.cheadrow[data-cdrop="${a.id}"]`));break;}
    case 'opentab':openTab(a.id);break;
    case 'opencol':S.wins.splice(S.focus+1,0,{tabs:[newTab(null,true),newTab(a.id)],a:1,hist:[]});S.focus++;break;
    case 'addchild':if(D[a.id].db){createPage(a.id,'New page');break;}{const id='new'+(++S.newN);D[id]={t:'page',n:'New page',w:'slim',toc:'marker',tags:[],kids:[],path:'not saved, this is a mockup',by:'you',ch:'now',vis:'As its parent',...none,stat:'new',html:'<p class="lab">Empty.</p>'};D[a.id].kids.unshift(id);S.tog[a.id]=true;rebuildPlaces();openDoc(id);S.just=id;break;}
    case 'copylink':S.toast='Link copied: '+D[a.id].n;setTimeout(()=>{S.toast='';render();},1600);break;
    case 'opengear':openDoc(a.id);at(S.focus).gear=true;break;
    case 'secup':case 'secdown':{const L=SORT[S.view],k=L.findIndex(x=>x.id===a.id),j=k+(act==='secup'?-1:1);if(k>=0&&j>=0&&j<L.length)[L[k],L[j]]=[L[j],L[k]];break;}
    case 'copen':onlyThis(a.id);break;
    case 'cpin':togglePin(a.id);break;
    case 'newcoll':{if(S.canCreate&&(!a.id||(D[a.id]&&D[a.id].real&&D[a.id].t==='coll'))){createCollection(a.id&&D[a.id]?a.id:'',freeName('New collection'));return;}const id='c'+(++S.newN),par=a.id&&D[a.id]?a.id:null;collDoc(id,freeName('New collection'),[],'grid');
      if(par){D[par].kids.push(id);S.tog[par]=true;rebuildPlaces();onlyThis(COLL[id].id);}else{C.push(asTop({id,i:'grid'}));onlyThis(id);}
      rebuildPlaces();S.view='workspace';S.rail=true;S.just=id;renameHere(id);return;}
    /* At the top, edit is what nothing set means; below, "as above" takes it from there. */
    case 'cacc':setAccess(a.c,a.id==='inherit'||(a.id==='edit'&&C.some(c=>c.id===a.c&&!c.alias))?'':a.id);return;
    case 'setacc':setAccess(t.d,a.id==='inherit'?'':a.id);return;
    case 'cmt':t.cmt=!t.cmt;t.cb=null;if(t.cmt)t.gear=false;break;
    case 'cblk':t.cmt=true;t.gear=false;t.cb=a.id;t.cbl=a.l||a.id;render();{const f=document.querySelector(`[data-addcmt="${i}"]`);if(f)f.focus();}return;
    case 'blocklink':S.toast='Link copied: /o/'+a.id+'. Pasted into another page, the table shows there and stays in one place.';setTimeout(()=>{S.toast='';render();},2600);break;
    case 'secclose':S.so[S.view][a.id]=false;if(S.spin[S.view])S.spin[S.view][a.id]=false;break;
    case 'aid':S.aid=!S.aid;break;
    case 'aiask':S.chat.push({q:a.id,a:aiAnswer()});break;
    case 'focus':S.focus=+a.id;break;
    case 'tog':S.tog[a.id]=!kidsOpen(a.id);break;
    case 'open':{const d=D[a.id],fold=d.t==='folder'||d.t==='dir'||d.kids.length>0;
      /* A click opens the object, whatever it is. A double click, or the arrow, opens or closes its children in nav3. */
      if(e.detail===2){if(fold&&b.closest('.nav3 .row'))S.tog[a.id]=!kidsOpen(a.id);break;}
      openDoc(a.id);break;}
    case 'coll':collClick(a.id,e);break;
    case 'collhead':S.open[a.id]=false;S.pin[a.id]=false;break;
    case 'n2line':case 'n2space':{const id=decoId(),k=C.findIndex(c=>c.id===a.id);decoDoc(id,act==='n2line'?'line':'space');C.splice(k<0?C.length:k+1,0,asTop({id,i:'grid'}));break;}
    case 'linefirst':case 'spacefirst':addDivider(act.startsWith('line')?'line':'space',a.id,'');break;
    case 'lineafter':case 'spaceafter':addDivider(act.startsWith('line')?'line':'space',contOf(a.id),a.id);break;
    case 'decodel':removeDivider(a.id);break;
    case 'delobj':if(!ctx0||!ctx0.ask){S.ctx={...ctx0,ask:true};break;}deleteObject(a.id);break;
    case 'trash':openDoc('trash:');loadTrash();break;
    case 'trrestore':restoreTrash(a.id);break;
    case 'trpurge':if(hadAsk!==a.id){S.trashAsk=a.id;break;}purgeTrash(a.id);break;
    case 'winmenu':{if(hadCtx==='win')break;const z=S.scale/100,r=b.getBoundingClientRect();S.ctx={kind:'win',x:Math.round(r.left/z),y:Math.round(r.bottom/z)+2};break;}
    case 'dupl':copyTo([a.id],contOf(a.id)||'',a.id);break;
    case 'tokind':convertKind(a.id,a.k);break;
    case 'show':if(!aliasOf(a.id))C.push(asAlias(a.id));break;
    case 'unshow':{const k=C.findIndex(c=>c.alias&&c.id===a.id);if(k>=0)C.splice(k,1);delete S.pin[a.id];delete S.open[a.id];break;}
    case 'sortaz':case 'sortza':sortKids(a.id,act==='sortaz'?1:-1);break;
    case 'selopen':if(S.sel){if(S.sel.z==='n2')S.sel.ids.forEach(x=>{if(!isDeco(x))S.open[x]=true;});else S.sel.ids.forEach(openAsTab);}S.sel=null;break;
    case 'selpin':if(S.sel)S.sel.ids.forEach(x=>{if(!isDeco(x)){S.pin[x]=true;S.open[x]=true;}});S.sel=null;break;
    case 'selclear':S.sel=null;break;
    /* Settings and name open the overview of the collection, where they are, ready to rename. */
    case 'cgear':{openDoc(a.id);render();const f=document.getElementById('cname');if(f){f.focus();f.select();}return;}
    case 'addmenu':if(hadAdd!==a.id)openNew(a.id,b.closest('.cheadrow,.n2head')||b);break;
    case 'lineend':case 'spaceend':{const k=D[a.id].kids;addDivider(act==='lineend'?'line':'space',a.id,k[k.length-1]||'');break;}
    case 'add':if(a.id==='page'&&D[a.c]&&D[a.c].db){S.addIn=null;createPage(a.c,'New page');break;}
      if(a.id==='folder'&&D[a.c]&&D[a.c].db){S.addIn=null;createFolder(a.c,freeName('New folder'));break;}{const id='new'+(++S.newN),k=a.id;D[id]={t:k,n:'New '+TYPE[k][1].toLowerCase(),w:'slim',toc:'marker',tags:[],kids:[],path:'not saved, this is a mockup',by:'you',ch:'now',vis:'As its collection',...none,stat:'empty',html:'<p class="lab">Empty. Drag it in the list to where it belongs.</p>'};C.find(c=>c.id===a.c).top.push(id);rebuildPlaces();openDoc(id);S.just=id;break;}
    case 's1':S.s1=+a.id;break;
    case 's2':S.s2=+a.id;break;
    case 'w3reset':S.w3=W3;break;
    case 'theme':S.theme=a.id;break;
    case 'scale':S.scale=+a.id===0?100:Math.min(150,Math.max(70,S.scale+ +a.id));break;
    case 'mod':S.mods[a.id]=!S.mods[a.id];break;
    case 'strat':S.strat=a.id;break;
    case 'ai':S.ai=!S.ai;break;
    case 'filters':S.filters=!S.filters;break;
    case 'recent':S.sq=a.id;break;
    case 'ftag':{const g=esc(a.tag);S.ftags=S.ftags.includes(g)?S.ftags.filter(x=>x!==g):[...S.ftags,g];break;}
    case 'tagsearch':if(S.view!=='search')S.prev=S.view;S.view='search';S.rail=true;S.sq='';S.ftype='';S.ftags=[esc(a.tag)];S.filters=true;break;
    case 'sel':w.a=tk;S.focus=i;break;
    case 'keep':{const c=take(w,0);w.tabs.push(c);w.a=w.tabs.length-1;break;}
    case 'close':if(w.tabs[tk].general)popGeneral(w);else{w.tabs.splice(tk,1);if(w.a>=tk&&w.a>0)w.a--;}compact();break;
    case 'menu':S.menu=hadMenu===i?null:i;break;
    case 'dup':{const s0=w.tabs[tk];w.tabs.splice(tk+1,0,{...s0,general:false,gear:false,fly:false});w.a=tk+1;break;}
    case 'move':placeTab(i,tk,+a.id,Infinity);break;
    case 'newwin':newColumn(i,tk,+a.id);break;
    case 'closeothers':closeWhere(i,j=>j===tk);break;
    case 'closeright':closeWhere(i,j=>j<=tk);break;
    case 'closeleft':closeWhere(i,j=>j>=tk);break;
    case 'closeall':closeWhere(i,()=>false);break;
    case 'closewin':S.wins.splice(i,1);if(!S.wins.length)S.wins=[{tabs:[newTab(null,true)],a:0,hist:[]}];S.focus=Math.min(Math.max(0,i-1),S.wins.length-1);break;
    case 'copypath':S.toast='Path copied: '+D[w.tabs[tk].d].path;setTimeout(()=>{S.toast='';render();},1600);break;
    case 'hist':S.hist=hadHist===i?null:i;break;
    case 'tablist':S.tlist=hadTl===i?null:i;break;
    case 'split':S.wins.splice(i+1,0,{tabs:[newTab(null,true),{...t,general:false,gear:false,fly:false}],a:1,hist:[]});S.focus=i+1;break;
    case 'gear':t.gear=!t.gear;if(t.gear){t.cmt=false;if(D[t.d].blocks)loadVersions(t.d);}break;
    case 'edit':t.edit=!t.edit;t.src=false;if(t.edit&&!D[t.d].blocks.length)addBlock(t.d,'');break;
    case 'blkadd':addBlock(t.d,a.id);return;
    case 'blkdel':deleteBlock(t.d,a.id);return;
    case 'src':t.src=!t.src;t.top=0;if(t.src)t.edit=false;break;
    case 'find':t.find=!t.find;if(t.find){render();const f=document.querySelector(`[data-findq="${i}"]`);if(f)f.focus();return;}break;
    case 'findnav':t.cur+=+a.id;render();{const c=document.querySelector(`[data-art="${i}"] mark.cur`);if(c)c.scrollIntoView({block:'center'});}return;
    case 'fly':t.fly=!t.fly;break;
    case 'jump':t.fly=false;render();{const x=document.getElementById(a.id);if(x)x.scrollIntoView({block:'start'});}return;
    case 'settoc':D[t.d].toc=a.id;break;
    case 'setw':D[t.d].w=a.id;break;
    case 'todo':{const it=D[t.d].items[+a.id];it.done=!it.done;break;}
    case 'slide':D[t.d].cur=+a.id;break;
    case 'clock':S.clock=!S.clock;break;
  }
  render();
},true);
function refreshHits(){const r=document.getElementById('sres');if(r)r.innerHTML=resultsHtml();}
document.addEventListener('input',e=>{
  if(e.target.dataset&&e.target.dataset.nfilter!==undefined){S.addQ=e.target.value;filterNew(S.addQ);return;}
  const x=e.target;
  if(x.dataset.blk!==undefined){const b=blockIn(x);if(b)b.draft=x.value;return;}
  if(x.id==='search'){S.sq=x.value;searchServer();if(S.view!=='search'){S.prev=S.view;S.view='search';S.rail=true;render();const f=document.getElementById('search');f.focus();f.setSelectionRange(f.value.length,f.value.length);}else refreshHits();}
  if(x.id==='cq'){S.cq=x.value;render();const f=document.getElementById('cq');if(f){f.focus();f.setSelectionRange(f.value.length,f.value.length);}}
  if(x.dataset.findq!==undefined){const i=+x.dataset.findq,t=at(i);t.q=x.value;t.cur=0;render();const f=document.querySelector(`[data-findq="${i}"]`);if(f){f.focus();f.setSelectionRange(f.value.length,f.value.length);}}
});
let rendering=false;
document.addEventListener('focusout',e=>{const x=e.target;if(!rendering&&x.dataset&&x.dataset.ren!==undefined)renameDone(x);});
document.addEventListener('change',e=>{
  const x=e.target;
  if(x.dataset.blk!==undefined){saveBlock(x.dataset.d,x.dataset.blk,x.value,x);return;}
  if(x.id==='limit'){S.limit=Math.max(0,+x.value||0);S.tog={};render();}
  if(x.id==='stype'){S.ftype=x.value;render();}
  if(x.dataset.cname!==undefined){const v=x.value.trim(),cid=x.dataset.cname;if(v){D[cid].n=esc(v);if(D[cid].real)renameObject(cid,v);}render();}
});
document.addEventListener('contextmenu',e=>{const tb=e.target.closest&&e.target.closest('.dtab[data-dw]');
  if(tb){e.preventDefault();const i=+tb.dataset.dw,sc=tb.closest('.tabscroll');S.ctx=null;S.menu=null;S.newAt=null;S.hist=null;S.tlist=null;S.focus=i;S.tmenu={w:i,t:+tb.dataset.dt,x:Math.max(4,tb.offsetLeft-sc.scrollLeft)};render();return;}
  const el=e.target.closest?e.target:null;if(!el)return;
  const z=S.scale/100,at0=(kind,id,more={})=>{e.preventDefault();S.menu=null;S.newAt=null;S.hist=null;S.tlist=null;S.tmenu=null;S.ctx={kind,id,x:Math.round(Math.min(e.clientX/z,innerWidth/z-250)),y:Math.round(Math.min(e.clientY/z,innerHeight/z-240)),...more};render();};
  const sl=S.sel&&!S.sel.soft&&S.sel.ids.length>1&&el.closest('[aria-selected="true"]');if(sl)return at0('sel',null);
  const cl=el.closest('[data-ctx="coll"]');if(cl)return at0('coll',cl.dataset.ctxid,{n3:!!cl.closest('.nav3')});
  const dc=el.closest('[data-ctx="deco"]');if(dc)return at0('deco',dc.dataset.ctxid);
  const rw=el.closest('[data-row]');if(rw)return at0('row',rw.dataset.row);
  const op=el.closest('.nav3 .row [data-act="open"]');if(op&&D[op.dataset.id])return at0('row',op.dataset.id);
  const sh=el.closest('.nav3 [data-sechead]');if(sh)return at0('sec',sh.dataset.sechead,{label:sh.textContent.trim(),sort:false});
  const sc=el.closest('.nav2 [data-act="sec"]');if(sc)return at0('sec',sc.dataset.id,{label:sc.textContent.trim(),sort:!!sc.dataset.secid});
  const vw=el.closest('.strip [data-act="view"]');if(vw)return at0('view',vw.dataset.id,{label:vw.textContent.trim()});
  /* A right click on the free part of nav2 or nav3 offers what can be made there. */
  if(el.closest('.nav2')){const acts=S.view==='workspace'?[['newcoll','New collection'],['n2line','Line at the end'],['n2space','Space at the end'],...(S.canCreate?[['-'],['trash','Trash']]:[])]:SECTIONS[S.view]?SECTIONS[S.view]().filter(x=>x[3]).map(x=>['sec:'+x[0],x[1]]):[];if(acts.length)return at0('free',null,{acts});}
  if(el.closest('.nav3')&&S.view==='workspace'){const acts=[...C.filter(c=>S.open[c.id]&&accOf(c.id)==='edit').map(c=>['addmenu2:'+c.id,'New in '+c.n]),['newcoll','New collection']];return at0('free',null,{acts});}
});
/* A middle click closes a tab, a double click on the empty part of the tab row opens New, as in code editors. */
document.addEventListener('auxclick',e=>{const op=e.button===1&&e.target.closest&&e.target.closest('.nav3 [data-act="open"]');if(op&&D[op.dataset.id]){e.preventDefault();openTab(op.dataset.id);render();return;}
  const tb=e.button===1&&e.target.closest&&e.target.closest('.dtab[data-dw]');if(!tb)return;e.preventDefault();const i=+tb.dataset.dw,k=+tb.dataset.dt,w=S.wins[i];
  if(w.tabs[k].general)popGeneral(w);else{w.tabs.splice(k,1);if(w.a>=k&&w.a>0)w.a--;}compact();render();});
document.addEventListener('dblclick',e=>{const r=e.target.closest&&e.target.closest('.tabrow');if(!r||e.target.closest('.dtab,button'))return;const i=+r.closest('[data-win]').dataset.win;S.focus=i;S.newAt=i;render();const f=document.querySelector('[data-quick]');if(f)f.focus();});
document.addEventListener('keydown',e=>{
  const x=e.target;
  if(x.dataset&&x.dataset.grip&&(e.key==='ArrowLeft'||e.key==='ArrowRight')){const n=x.dataset.grip,dir=e.key==='ArrowLeft'?-1:1;if(n==='3')S.w3=clampW3(S.w3+dir*20);else{const k='s'+n;S[k]=Math.min(3,Math.max(0,S[k]+dir));}render();const g=document.querySelector(`[data-grip="${n}"]`);if(g)g.focus();e.preventDefault();return;}
  if(e.key==='/'&&!x.closest('input,select,textarea')){e.preventDefault();if(!document.getElementById('search')){S.view='search';S.rail=true;render();}document.getElementById('search').focus();return;}
  if(x.dataset&&x.dataset.nfilter!==undefined&&['ArrowDown','ArrowUp','Enter'].includes(e.key)){e.preventDefault();if(e.key==='Enter'){const n=document.querySelector('.cmdpop [role="menuitem"].active')||[...document.querySelectorAll('.cmdpop [role="menuitem"]')].find(n=>n.getAttribute('aria-hidden')!=='true'&&!n.disabled);if(n)n.click();}else stepNew(e.key==='ArrowDown'?1:-1);return;}
  if(x.dataset&&x.dataset.ren!==undefined&&(e.key==='Enter'||e.key==='Escape')){e.preventDefault();if(e.key==='Escape')x.value=x.defaultValue;x.blur();return;}
  if(e.key==='Escape'&&(S.newAt!==null||S.ctx||S.addIn||S.tmenu||S.hist!==null||S.tlist!==null)){S.newAt=null;S.ctx=null;S.addIn=null;S.tmenu=null;S.hist=null;S.tlist=null;render();return;}
  if(e.key==='Escape'&&S.sel&&!S.sel.soft&&!drag){S.sel=null;render();return;}
  if(x.dataset&&x.dataset.blk!==undefined&&e.key==='Enter'&&(e.ctrlKey||e.metaKey)){e.preventDefault();x.blur();return;}
  if(x.dataset&&x.dataset.blk!==undefined&&e.key==='Escape'){const b=blockIn(x);if(b){x.value=b.raw;delete b.draft;}x.blur();return;}
  if(e.key!=='Enter')return;
  if(x.dataset.quick!==undefined){const v=x.value.trim();if(!v)return;S.newAt=null;if(S.into==='qn'){D.qn.notes.unshift({h:'',b:v});openDoc('qn');}else if(D[S.into]&&D[S.into].db)createPage(S.into,v.split('\n')[0].slice(0,200));else openDoc(createIn(S.into,'page',v));render();return;}
  if(x.dataset.aiq!==undefined){const v=x.value.trim();if(!v)return;S.chat.push({q:v,a:aiAnswer()});render();const n=document.querySelector('[data-aiq]');if(n)n.focus();return;}
  if(x.id==='search'){const h=hitsWithServer();if(h&&h.length){openDoc(h[0]);render();const f=document.getElementById('search');if(f)f.focus();}}
  if(x.dataset.addtag!==undefined&&x.dataset.cname===undefined){const v=x.value.trim().toLowerCase().replace(/^#/,''),d=D[at(+x.dataset.addtag).d];if(v&&!d.tags.includes(esc(v)))d.tags.push(esc(v));render();}
  if(x.dataset.addcmt!==undefined){const v=x.value.trim(),d=D[at(+x.dataset.addcmt).d];const tt=at(+x.dataset.addcmt);if(v&&tt.cb)(d.comments=d.comments||[]).push({b:tt.cb,l:tt.cbl,who:'A. Keller',when:'now',t:v});render();const n=document.querySelector('[data-addcmt]');if(n)n.focus();return;}
  if(x.dataset.addnote!==undefined){const v=x.value.trim(),d=D[at(S.focus).d];if(v)d.notes.unshift({h:'',b:v});render();const n=document.querySelector('[data-addnote]');if(n)n.focus();}
  if(x.dataset.findq!==undefined){const i=+x.dataset.findq,t=at(i);t.cur++;render();const c=document.querySelector(`[data-art="${i}"] mark.cur`);if(c)c.scrollIntoView({block:'center'});const f=document.querySelector(`[data-findq="${i}"]`);if(f)f.focus();}
});
/* Dragging runs on pointer events: it starts anywhere on the item, also on its text, and the whole row counts as the target. */
let drag=null,noClick=false;
const clampW3=v=>Math.min(640,Math.max(180,Math.round(v)));
const clearMarks=()=>document.querySelectorAll('.zone-left,.zone-right,.zone-mid,.zone-open').forEach(n=>n.classList.remove('zone-left','zone-right','zone-mid','zone-open'));
function startGrip(g,e){
  const n=g.dataset.grip,z=S.scale/100,x0=e.clientX,w0=n==='3'?S.w3:STAGES[S['s'+n]];
  const move=m=>{const want=w0+(m.clientX-x0)/z;
    if(n==='3'){S.w3=clampW3(want);document.documentElement.style.setProperty('--w3',S.w3+'px');return;}
    let best=0;STAGES.forEach((w,k)=>{if(Math.abs(w-want)<Math.abs(STAGES[best]-want))best=k;});
    if(best!==S['s'+n]){S['s'+n]=best;render();}};
  const up=()=>{removeEventListener('pointermove',move);removeEventListener('pointerup',up);document.body.classList.remove('resizing');render();};
  document.body.classList.add('resizing');addEventListener('pointermove',move);addEventListener('pointerup',up);e.preventDefault();
}
document.addEventListener('pointerdown',e=>{
  noClick=false;
  if(e.button!==0||!e.target.closest)return;
  const g=e.target.closest('[data-grip]');
  if(g){startGrip(g,e);return;}
  if(e.pointerType==='touch'||e.target.closest('.mini,.arrow,.star,.addbtn,.cmore,.pop,input,textarea,select'))return;
  const tab=e.target.closest('.dtab:not(.blank)');
  if(tab)drag={kind:'tab',src:tab,w:+tab.dataset.dw,t:+tab.dataset.dt,label:tab.querySelector('.sel span').textContent};
  if(drag){drag.x0=e.clientX;drag.y0=e.clientY;drag.on=false;drag.to=null;}
});
/* Tabs snap: the window comes from the pointer's x, the row has a tolerance around it, and the tab takes its place while dragging. */
function winAt(e){
  return [...document.querySelectorAll('[data-win]')].find(w=>{const r=w.getBoundingClientRect();return e.clientX>=r.left&&e.clientX<r.right&&e.clientY>=r.top-24&&e.clientY<=r.bottom;});
}
function tabTarget(e){
  const w=winAt(e);if(!w)return null;
  const i=+w.dataset.win,row=w.querySelector('.tabrow'),rr=row.getBoundingClientRect();
  if(e.clientY<=rr.bottom+10){
    const tabs=[...row.querySelectorAll('.dtab:not(.general):not(.slot)')].filter(n=>!(+n.dataset.dw===drag.w&&+n.dataset.dt===drag.t));
    /* The general tab stays while it is dragged, so its slot adds width. Measure as if the slot were not there, or it pushes its own target away. */
    const slot=drag.src.classList.contains('general')&&row.querySelector('#slot'),sw=slot?slot.getBoundingClientRect().width+2:0;
    let seq=tabs.findIndex(n=>{const r=n.getBoundingClientRect(),sh=slot&&(slot.compareDocumentPosition(n)&Node.DOCUMENT_POSITION_FOLLOWING)?sw:0;return e.clientX<r.left-sh+r.width/2;});
    if(seq<0)seq=tabs.length;
    return {type:'row',i,seq,idx:seq+1,row,before:tabs[seq]||null};
  }
  const r=w.getBoundingClientRect(),band=r.width*.3;
  if(S.wins.length<4){if(e.clientX<r.left+band)return {type:'new',pos:i,node:w,cls:'zone-left'};if(e.clientX>r.right-band)return {type:'new',pos:i+1,node:w,cls:'zone-right'};}
  return i===drag.w?null:{type:'win',i,node:w,cls:'zone-mid'};
}
function showSlot(t){
  let s=document.getElementById('slot');
  if(!t||t.type!=='row'){if(s)s.remove();return false;}
  if(!s){s=drag.src.cloneNode(true);s.id='slot';s.style.display='';s.classList.remove('general');s.classList.add('slot','on');s.removeAttribute('data-dw');s.removeAttribute('data-dt');}
  const box=t.row.querySelector('.tabscroll');
  if(t.before)box.insertBefore(s,t.before);else box.append(s);
  return true;
}
const within=(id,a)=>{for(let x=a;x;x=PARENT[x])if(x===id)return true;return false;};
/* Where a drop puts the entry: the container it goes into. An object of the database goes only into the database, where
   one may edit, and made-up content stays made up. */
function dropCont(t){return t.type==='coll'?t.c:t.type==='into'?t.id:t.type==='nav2'?'':contOf(t.id);}
function fits(id,t){const c=dropCont(t);if(!D[id].db)return c===''||!D[c]||!D[c].db;if(c==='')return D[id].t==='coll';return !!D[c]&&!!D[c].db&&accOf(c)==='edit'&&!isDeco(c);}
/* Dragging an entry of nav2 or nav3 is C22's (app-workspace-drag.js, approved here on 2026-11-09): it carries the entry
   through the list as it looks when held, up and down between entries, onto one after waiting, deeper to the right. What
   may go where, and what a drop does, is decided here. The entries selected along with it go too; on a collection in
   nav2 an entry goes into it, on a window it opens there as tabs, and with Ctrl held at the drop it is copied. */
const selOf=kind=>kind==='row'?'n3':kind==='fav'?'n2':'sec';
const elOf=(kind,id)=>document.querySelector(kind==='row'?`.nav3 [data-row="${id}"]`:kind==='fav'?`#clist [data-favid="${id}"]`:`.nav2 [data-secid="${id}"]`);
const kindOfEl=n=>n.matches('[data-row]')?'row':n.matches('[data-favid]')?'fav':'sec';
function depthOf(n){let k=-1;for(let x=n.parentElement;x&&x.id!=='tree';x=x.parentElement)if(x.matches('ul.t'))k++;return k;}
/* May every dragged entry go into container c? */
function mayInto(d,c,head){
  if(accOf(c)==='read')return false;const probe=head?{type:'coll',c}:{type:'into',id:c};
  return d.ids.every(x=>fits(x,probe)&&!(d.kind==='fav'&&(c===x||aliasOf(x)||(COLL[c]&&COLL[c].id===x)))&&!(d.kind==='row'&&within(x,c)));
}
const DRAG={
  scale:()=>S.scale/100,
  strip:['data-row','data-favid','data-secid'],
  pick(e){
    if(!e.target.closest||e.target.closest('.mini,.arrow,.star,.addbtn,.cmore,.pop,input,textarea,select,[data-grip]'))return null;
    const el=e.target.closest('[data-row],[data-favid],[data-secid]');if(!el||el.closest('.dropgap'))return null;
    const kind=kindOfEl(el),id=el.dataset.row||el.dataset.favid||el.dataset.secid;
    if(kind==='row'&&contAcc(id)==='read')return null;
    /* An entry of a selection takes the others along, in the order of the list; in nav3 one below another selected one
       goes with that one anyway, and what may only be read stays. */
    const many=S.sel&&S.sel.z===selOf(kind)&&S.sel.ids.includes(id)?S.sel.ids.filter(x=>elOf(kind,x)&&(kind!=='row'||contAcc(x)!=='read')):[id];
    const ids=kind==='row'?many.filter(x=>!many.some(y=>y!==x&&within(y,PARENT[x]))):many;
    ids.sort((a,b)=>elOf(kind,a).compareDocumentPosition(elOf(kind,b))&Node.DOCUMENT_POSITION_FOLLOWING?-1:1);
    return {kind,id,ids,view:S.view,els:ids.map(x=>elOf(kind,x)),el};
  },
  again(d){const el=elOf(d.kind,d.id);return el?{el,els:d.ids.map(x=>elOf(d.kind,x)).filter(Boolean)}:null;},
  hideOf:el=>el.matches('[data-row]')?el.closest('li'):el,
  home:d=>selOf(d.kind),
  depthOf,
  zones(d){
    const z=S.scale/100,out=[],n2=document.querySelector('.nav2'),list=document.getElementById('clist');
    if(list&&d.kind!=='sec')out.push({key:'n2',box:n2,scroll:list,flat:true,rows:[...list.querySelectorAll(':scope>.collrow[data-favid]')].map(n=>({el:n,id:n.dataset.favid,hold:!isDeco(n.dataset.favid)}))});
    if(d.kind==='sec'&&n2)out.push({key:'sec',box:n2,scroll:n2,flat:true,rows:[...n2.querySelectorAll('[data-secid]')].map(n=>({el:n,id:n.dataset.secid}))});
    const tr=document.getElementById('tree'),top=tr&&tr.querySelector('ul.t.top');
    if(top&&(d.kind==='row'||(d.kind==='fav'&&!d.ids.some(x=>isDeco(x)||aliasOf(x))))){
      const p=document.createElement('ul');p.className='t';top.append(p);const indent=parseFloat(getComputedStyle(p).paddingLeft)*z;p.remove();
      out.push({key:'n3',box:tr.closest('.nav3'),scroll:tr,flat:false,base:top.getBoundingClientRect().left,indent,
        rows:[...tr.querySelectorAll('.cheadrow,.row')].filter(n=>!n.closest('.dropgap')).map(n=>n.classList.contains('cheadrow')?{el:n,id:n.dataset.cdrop,head:true,depth:-1,hold:true}:{el:n,id:n.dataset.row,depth:depthOf(n),hold:!isDeco(n.dataset.row)})});
    }
    return out;
  },
  /* In nav2 an entry that is not a collection can only go into one: at once, wherever on it. */
  ontoOnly:(d,z)=>z.key==='n2'&&d.kind==='row'&&!d.ids.every(x=>D[x].t==='coll'),
  waits:z=>z.key!=='sec',
  accept(d,z,at){
    if(at.onto){const c=at.onto.id;if(!mayInto(d,c,at.onto.head))return null;const kids=D[c].kids.filter(x=>!d.ids.includes(x));return {p:c,after:kids[kids.length-1]||''};}
    const after=at.after?at.after.id:'';
    if(z.key==='sec')return {after};
    if(z.key==='n2')return d.ids.every(x=>d.kind==='fav'||fits(x,{type:'nav2'}))?{root:true,after}:null;
    return mayInto(d,at.parent.id,at.parent.head)?{p:at.parent.id,after}:null;
  },
  outside(d,e){if(d.kind==='sec'||d.ids.some(isDeco))return null;const win=winAt(e);return win?{win:+win.dataset.win,node:win}:null;},
  drop(d,t,e){d.copy=!!(e&&(e.ctrlKey||e.metaKey));applyDrop(d,t);},
  end(){render();}
};
/* Tabs stay this app's own: a label at the pointer, and the tab takes its place in a row or a new window. */
addEventListener('pointermove',e=>{
  if(!drag)return;
  const z=S.scale/100;
  if(!drag.on){if(Math.hypot(e.clientX-drag.x0,e.clientY-drag.y0)<6)return;drag.on=true;document.body.classList.add('dragging');
    const g=document.createElement('div');g.id='ghost';g.className='ghost';g.textContent=drag.label;document.body.append(g);if(!drag.src.classList.contains('general'))drag.src.style.display='none';}
  const g=document.getElementById('ghost');
  g.style.left=(e.clientX/z+12)+'px';g.style.top=(e.clientY/z+12)+'px';
  drag.to=tabTarget(e);clearMarks();const t=drag.to;
  g.style.display=showSlot(t)?'none':'';
  if(t&&t.node)t.node.classList.add(t.cls);
});
function parentList(id){return PARENT[id]?D[PARENT[id]].kids:COLL[id]?COLL[id].top:C.find(c=>c.id===id)?[]:[];}
/* A drop takes the entries where they are held, in their order, or copies them there with Ctrl. In the database the
   server moves them too, one after the other, and if it refuses, the lists go back. */
function applyDrop(d,t=d.to){
  if(!t)return;
  if(d.kind==='tab'){
    if(t.type==='row'){if(t.i!==d.w||t.idx!==d.t)placeTab(d.w,d.t,t.i,t.idx);else{S.wins[d.w].a=d.t;S.focus=d.w;}}
    else if(t.type==='new')newColumn(d.w,d.t,t.pos);
    else placeTab(d.w,d.t,t.i,Infinity);return;}
  const ids=d.ids;
  if(t.win!==undefined){S.focus=t.win;ids.forEach(openAsTab);return;}
  const w=t.zone.key;
  if(w==='sec'){const L=SORT[d.view],ms=ids.map(id=>L.splice(L.findIndex(x=>x.id===id),1)[0]);L.splice(t.after?L.findIndex(x=>x.id===t.after)+1:0,0,...ms);return;}
  if(d.copy){copyTo(ids.filter(x=>!isDeco(x)||D[x].db),t.root?'':t.p,t.after);return;}
  const dest=t.root?C:D[t.p].kids,froms=ids.map(id=>d.kind==='fav'?C:parentList(id)),snap=[...new Set([C,dest,...froms])].map(l=>[l,l.slice()]);
  const ms=ids.map((id,i)=>{if(d.kind==='fav')return C.splice(C.findIndex(c=>c.id===id),1)[0];froms[i].splice(froms[i].indexOf(id),1);return null;});
  if(t.root)C.splice(t.after?C.findIndex(c=>c.id===t.after)+1:0,0,...ids.map((id,i)=>{if(ms[i])return ms[i];S.open[id]=true;return asTop({id,i:D[id].i||'grid'});}));
  else{ids.forEach((id,i)=>{if(ms[i]){delete S.open[id];delete S.fav[id];delete S.pin[id];}});dest.splice(t.after?dest.indexOf(t.after)+1:0,0,...ids);S.tog[t.p]=true;S.just=d.id;}
  rebuildPlaces();
  /* nav2 keeps its order in the browser; only a move into or out of a collection reaches the server. */
  if(d.kind==='fav'&&t.root)return;
  const moves=ids.filter(id=>D[id].db).map(id=>[id,t.root?'':t.p,t.root?'':(ids.indexOf(id)?ids[ids.indexOf(id)-1]:t.after)]);
  moveAll(moves,()=>{snap.forEach(([l,c])=>{l.length=0;l.push(...c);});rebuildPlaces();render();});
}
function openAsTab(id){if(!D[id]||isDeco(id))return;openTab(id);if(D[id].real&&!D[id].loaded)loadDoc(id);}
addEventListener('pointerup',e=>{
  if(!drag)return;const d=drag;drag=null;
  if(!d.on)return;
  noClick=true;setTimeout(()=>{noClick=false;},60);
  document.body.classList.remove('dragging');const g=document.getElementById('ghost');if(g)g.remove();clearMarks();
  applyDrop(d);render();
});
addEventListener('keydown',e=>{if(e.key==='Escape'&&drag){const g=document.getElementById('ghost');if(g)g.remove();const on=drag.on;drag=null;document.body.classList.remove('dragging');clearMarks();if(on)render();}});
/* Documents that need the helpers above */
const fill=o=>{const def={w:'slim',toc:'marker',tags:[],kids:[],vis:'Only you',...none};for(const k in def)if(!(k in o))o[k]=def[k];return o;};
D['priv:time']=fill({t:'time',n:'This week',w:'wide',path:'not a file, private',place:'Private › Time',by:'you',ch:'today',stat:'31:40 this week',
  get html(){return `<button class="clockbtn" data-act="clock" aria-pressed="${S.clock}">${ic('clock')}${S.clock?'Clock out, running since 08:02':'Clock in'}</button>${tableHtml(['Day','In','Out','Break','Hours'],[['Mon','07:58','16:40','0:30','8:12'],['Tue','08:05','17:02','0:30','8:27'],['Wed','08:00','16:15','0:30','7:45'],['Thu','08:10','15:56','0:30','7:16']])}`;}});
D['priv:vac']=fill({t:'vacation',n:'Vacation 2026',w:'wide',path:'not a file, private',place:'Private › Vacation',by:'you',ch:'Oct 30',stat:'7 days left',
  html:`<p>30 days this year: 18 taken, 5 planned, 7 left.</p>${tableHtml(['From','To','Days','State'],[['Mar 02','Mar 13','10','✓ taken'],['Jul 20','Jul 27','8','✓ taken'],['Nov 24','Nov 28','5','△ planned']])}`});
const box=x=>x.done?ic('check'):'<span class="box"></span>';
function todoDocs(){TL.forEach(l=>{
  D['tl:'+l.id]=fill({t:'todo',n:l.n,path:'not a file, private',place:'To-do',by:'you',ch:'today',
    get stat(){return `${l.items.filter(x=>x.done).length} of ${l.items.length} done`;},
    get html(){return `<div class="todos">${l.items.map((x,k)=>`<button class="todo${x.done?' done':''}" data-act="tick" data-l="${l.id}" data-id="${k}" aria-pressed="${x.done}">${box(x)}<span>${esc(x.t)}</span>${x.due?`<em class="due">${x.due}</em>`:''}</button>`).join('')||'<p class="lab">Empty. Use + in nav3.</p>'}</div>`;}});
  l.items.forEach((x,k)=>{D[`ti:${l.id}/${k}`]=fill({t:'todo',n:x.t,path:'not a file, private',place:'To-do › '+l.n,by:'you',ch:'today',get stat(){return x.done?'done':'open';},
    get html(){return `<button class="todo${x.done?' done':''}" data-act="tick" data-l="${l.id}" data-id="${k}" aria-pressed="${x.done}">${box(x)}<span>${esc(x.t)}</span></button><dl class="mailhead" data-style="margin-top:12px"><dt>Due</dt><dd>${x.due||'–'}</dd><dt>List</dt><dd>${esc(l.n)}</dd></dl>`;}});});
});}
todoDocs();
const calGrid=cs=>{const ev={};cs.forEach(c=>Object.entries(c.ev).forEach(([d,n])=>(ev[d]=ev[d]||[]).push(n)));
  return `<div class="calgrid">${['Mon','Tue','Wed','Thu','Fri','Sat','Sun'].map(x=>`<b>${x}</b>`).join('')}${'<span></span>'.repeat(6)}${Array.from({length:30},(_,k)=>k+1).map(n=>`<div class="day${n===7?' today':''}"><em>${n}</em>${(ev[n]||[]).map(x=>`<small>${x}</small>`).join('')}</div>`).join('')}</div>`;};
[['all','All calendars',CALS],...CALS.map(c=>[c.id,c.n,[c]])].forEach(([id,n,cs])=>{D['cal:'+id]=fill({t:'cal',n:n+', November 2026',w:'wide',path:'not a file, private',place:'Calendar',by:'you',ch:'today',stat:cs.reduce((a,c)=>a+Object.keys(c.ev).length,0)+' entries',html:calGrid(cs)+'<p class="hint">Shared calendars from documents come with M-18.</p>'});});
CALS.forEach(c=>Object.entries(c.ev).forEach(([d,n])=>{D[`ev:${c.id}/${d}`]=fill({t:'cal',n,path:'not a file, private',place:'Calendar › '+c.n,by:'you',ch:'today',stat:'1 entry',html:`<dl class="mailhead"><dt>When</dt><dd>Nov ${d}, 2026</dd><dt>Calendar</dt><dd>${c.n}</dd></dl>`});}));
D['cal:import']=fill({t:'cal',n:'Import a calendar',w:'wide',path:'not a file',place:'Calendar › Import',by:'you',ch:'now',stat:'nothing imported',html:`<div class="dropzone">${ic('down')}<span>Drop a calendar file here</span><button class="chip" disabled>Choose a file</button></div><input class="addtag" disabled placeholder="Or the address of a calendar to keep reading from" aria-label="Calendar address"><p class="hint">An imported calendar appears in nav2 next to your own, and can be sorted there.</p>`});
D['time:last']=fill({t:'time',n:'Last week',w:'wide',path:'not a file, private',place:'Time',by:'you',ch:'Nov 01',stat:'38:35',html:tableHtml(['Day','In','Out','Break','Hours'],[['Mon','08:00','16:30','0:30','8:00'],['Tue','07:55','16:30','0:30','8:05'],['Wed','08:00','17:00','0:30','8:30'],['Thu','08:10','16:30','0:30','7:50'],['Fri','08:00','14:40','0:30','6:10']])});
D['time:oct']=fill({t:'time',n:'October',w:'wide',path:'not a file, private',place:'Time',by:'you',ch:'Nov 01',stat:'157:35',html:tableHtml(['Week','Hours'],[['40','39:10'],['41','40:05'],['42','38:20'],['43','40:00']])});
D['time:export']=fill({t:'time',n:'Export the records',path:'not a file',place:'Time',by:'you',ch:'now',stat:'later',html:'<p class="hint">As a table, for a chosen period. Whether records feed anything official needs its own look first (M-24).</p>'});
D['vac:27']=fill({t:'vacation',n:'Vacation 2027',w:'wide',path:'not a file, private',place:'Vacation',by:'you',ch:'now',stat:'30 days left',html:'<p>30 days, none planned yet.</p>'});
D['vac:new']=fill({t:'vacation',n:'Plan a vacation',path:'not a file, private',place:'Vacation',by:'you',ch:'now',stat:'draft',html:'<p class="hint">From, to and a note. A request to someone else is a question for M-24.</p>'});
const boardDoc=(n,notes)=>fill({t:'notes',n,w:'full',path:'not a file, private notes',place:'Notes › '+n,by:'you',ch:'today',notes,get stat(){return `${this.notes.length} notes`;},
  get html(){return `<input class="noteadd" data-addnote placeholder="Take a note, then Enter" aria-label="Take a note"><div class="keep">${this.notes.map(x=>`<div class="kard">${x.h?`<b>${esc(x.h)}</b>`:''}<span>${esc(x.b).replace(/\n/g,'<br>')}</span></div>`).join('')}</div>`;}});
D['nb:ideas']=boardDoc('Ideas',[{h:'Start page',b:'Show the tower height.'},{h:'',b:'A map of the site.'}]);
Object.assign(TYPE,{ipnet:['net','IP range'],devices:['hd','Device list'],cal:['cal','Calendar'],time:['clock','Time'],vacation:['sun','Vacation']});
const netDoc=(n,used,rows,txt)=>fill({t:'ipnet',n,w:'wide',path:'not a file, lives in the database',by:'A. Keller',ch:'Oct 28',vis:'Group ops, inherited',tags:['network'],stat:used+' used',html:`<p>${txt} The page <button class="l" data-act="open" data-id="plan">Network plan</button> can embed this range, and a value can come from an address.</p>`+tableHtml(['Address','Name','Kind','Note'],rows)});
D.ip1=netDoc('192.0.2.0/24','4 of 254',[['192.0.2.1','router','Router','Value router.ip'],['192.0.2.2','core-a','Switch',''],['192.0.2.3','core-b','Switch',''],['192.0.2.20','printer office','Printer','']],'Office.');
D.ip2=netDoc('198.51.100.0/24','2 of 254',[['198.51.100.5','camera gate','Camera',''],['198.51.100.6','sensor tower','Sensor','']],'Devices.');
D.devs=fill({t:'devices',n:'Network devices',w:'wide',path:'not a file, lives in the database',by:'A. Keller',ch:'Oct 28',vis:'Group ops, inherited',tags:['hardware'],stat:'3 devices',html:tableHtml(['Name','Kind','Room','Ports','Address'],[['router','Router','Server room','8','192.0.2.1'],['core-a','Switch','Server room','48','192.0.2.2'],['core-b','Switch','Workshop','48','192.0.2.3']])});
D.net.kids=[...D.net.kids,'ip1','ip2','devs'];
TYPE.search=['search','Saved search'];
const savedHits=o=>Object.keys(D).filter(id=>{const d=D[id];if(id.includes(':')||d.t==='search'||d.gone)return false;if(o.type&&d.t!==o.type)return false;if(!o.ftags.every(t=>(d.tags||[]).includes(t)))return false;return !o.q||docText(d).toLowerCase().includes(o.q.toLowerCase());});
/* A saved search is a document: a query with filters and sources that runs again each time it opens. */
D.ss1=fill({t:'search',n:'Everything on the network',w:'wide',path:'not a file, a saved search',by:'A. Keller',ch:'Oct 30',vis:'Group ops, inherited',desc:'All documents tagged network, wherever they are.',q:'',ftags:['network'],type:'',strat:'Full text',src:['Workspace','Share','Ticket system'],
  get stat(){return savedHits(this).length+' hits';},
  get html(){const h=savedHits(this);return `<div class="ssbar"><span class="lab">Query</span><span class="chip">${this.q?esc(this.q):'any text'}</span><span class="lab">Strategy</span><span class="chip">${this.strat}</span><span class="lab">Filter</span>${this.ftags.map(t=>`<span class="tagc">#${t}</span>`).join('')}${this.type?`<span class="chip">${TYPE[this.type][1]}</span>`:''}<span class="lab">Sources</span>${this.src.map(x=>`<span class="chip">${x}</span>`).join('')}</div>
    <p class="hint">Runs again each time it opens. Changing the query comes with editing (M-8), and the strategies with M-12.</p>
    <div class="sshits">${h.map(x=>`<button class="hit" data-act="open" data-id="${x}">${ic(TYPE[D[x].t][0])}<span>${D[x].n}<small>${placeOf(x)}</small></span></button>`).join('')}</div>`;}});
C.find(c=>c.id==='ops').top.push('ss1');
TYPE.bookmark=['globe','Bookmark'];TYPE.news=['globe','News entry'];
BM.forEach(f=>f.items.forEach((b,k)=>{D[`bm:${f.id}/${k}`]=fill({t:'bookmark',n:b.n,path:'not a file, a bookmark',place:'Bookmarks › '+f.n,by:'you',ch:'Oct 20',stat:'1 link',tags:f.id==='work'?['hardware']:[],
  html:`<p><a href="${b.u}" target="_blank" rel="noopener noreferrer">${b.u}</a></p><p>${b.d}</p><div class="note">Nothing is loaded from the site, so no preview and no icon reach a third party. The link opens in a new browser tab.</div>`});}));
D['bm:import']=fill({t:'bookmark',n:'Import bookmarks',w:'wide',path:'not a file',place:'Bookmarks › Import',by:'you',ch:'now',stat:'nothing imported',
  html:`<div class="dropzone">${ic('down')}<span>Drop the HTML file of your browser here</span><button class="chip" disabled>Choose a file</button></div><p class="hint">Folders stay folders and links already there are skipped. Firefox, Chrome, Edge and Safari all export this format.</p>`});
NEWS.forEach(n=>{D['news:'+n.id]=fill({t:'news',n:n.h,path:'not a file, from a feed',place:'News › '+FEEDS.find(x=>x.id===n.f).n,by:FEEDS.find(x=>x.id===n.f).n,ch:n.date,stat:'1 entry',html:`<p>${n.b}</p><div class="note">Save it as a document, or send it to a collection, comes with the news module.</div>`});});
D['news:sub']=fill({t:'news',n:'Subscribe to a feed',path:'not a file',place:'News › Subscribe',by:'you',ch:'now',stat:'later',html:`<input class="addtag" disabled placeholder="Address of an RSS or Atom feed" aria-label="Feed address"><p class="hint">Later: feeds, and maybe pages that change. Every user has their own.</p>`});
D.plan.comments=[{b:'h3',l:'Radio link',who:'J. Roth',when:'Oct 29',t:'Is the radio link backed up by a second path?'},{b:'h3',l:'Radio link',who:'M. Sander',when:'Oct 30',t:'Not yet, it is on the list for spring.'},{b:'t1',l:'Table: Name, Room, Uplink',who:'A. Keller',when:'Nov 02',t:'The uplink of core-b is 1 Gbit for now.'}];
/* Bookmarks are objects: a folder of links in Operations. The import of a browser file comes from New at a collection. */
D.links=fill({t:'folder',n:'Links',kids:['bk1','bk2','bk3'],path:'not a file, a folder in the workspace',by:'A. Keller',ch:'Oct 20',vis:'Everyone signed in'});
[['bk1','Ticket system','https://tickets.example.com','Open tickets of the team.'],['bk2','Supplier portal','https://supplier.example.net','Orders and licenses.'],['bk3','Switch manual','https://docs.example.com/switch','Firmware 9, chapter on VLANs.']].forEach(([id,n,u,dsc])=>{D[id]=fill({t:'bookmark',n,path:'not a file, a bookmark',by:'A. Keller',ch:'Oct 20',vis:'Everyone signed in',desc:dsc,stat:'1 link',
  html:`<p><a href="${u}" target="_blank" rel="noopener noreferrer">${u}</a></p><div class="note">Nothing is loaded from the site, so no preview and no icon reach a third party. The link opens in a new browser tab.</div>`});});
D.ops.kids.splice(D.ops.kids.indexOf('ss1'),0,'links');D['bm:import'].place='Workspace';
D.rack=fill({t:'page',n:'Rack overview',w:'wide',path:'operations/network/rack.md',by:'A. Keller',ch:'Nov 03',vis:'Everyone signed in',tags:['hardware'],stat:'1 embedded table',desc:'What sits in the rack, with the switch table of the network plan.',
  get html(){const sec=D.plan.doc.secs.find(x=>x[0]==='Core switches'),tb=(sec[1].match(/<div class="tw">[\s\S]*?<\/table><\/div>/)||[''])[0];
    return `<p>The table below is not a copy. It lives in the network plan and shows here, so a change there shows here too.</p><div class="embed"><div class="embhead">${ic('opennew')}<span>Core switches, from <button class="l" data-act="open" data-id="plan">Network plan</button></span><em>/o/plan#t1</em></div>${tb}</div><p class="hint">Paste as a copy instead comes later. A copy gets IDs of its own and no longer follows the source.</p>`;}});
D.net.kids.push('rack');D.plan.usedIn=[...(D.plan.usedIn||[]),'Rack overview (table #t1)'];
/* ---- UnderDoc: the real data in the workspace of the mockup (sprint 2026-11-08) ----
   Who is signed in, the collections and their trees come from /api/state; a page loads from /api/doc/<id> when it opens.
   What has no backend yet keeps the made-up content of the mockup, marked as a demo. */

/* Strict CSP: markup carries data-style, and this puts it into the CSSOM, which the CSP allows. */
const applyStyles=root=>{if(!root||!root.querySelectorAll)return;if(root.dataset&&root.dataset.style!==undefined)root.style.cssText=root.dataset.style;root.querySelectorAll('[data-style]').forEach(n=>{n.style.cssText=n.dataset.style;});};
new MutationObserver(records=>records.forEach(r=>{r.addedNodes.forEach(applyStyles);if(r.type==='attributes')applyStyles(r.target);})).observe(document.documentElement,{childList:true,subtree:true,attributes:true,attributeFilter:['data-style']});

/* Demo content: the made-up collections stay below the real ones, marked. */
C.forEach(c=>{D[c.id].n=D[c.id].n+' (demo)';S.fav[c.id]=false;});
S.open={};S.pin={};

async function loadDoc(id){
  const d=D[id];if(!d||d.loading||d.t!=='page')return;d.loading=true;d.html='<p class="lab">Loading…</p>';
  try{
    const r=await fetch('/api/doc/'+pathOf(d.sid||id),{credentials:'same-origin',headers:{Accept:'application/json'}});
    if(!r.ok)throw new Error(String(r.status));
    const a=await r.json();
    delete d.html;
    /* Names and texts from the server are escaped once, here; the rendered HTML of a page comes clean from the server. */
    d.doc={intro:a.intro,secs:a.secs.map(x=>[esc(x[0]),x[1]])};d.desc=esc(a.desc);d.tags=a.tags.map(esc);d.path=esc(a.path);d.ch=esc(a.ch);d.by=d.by||'—';d.vis='Everyone signed in';
    d.usedIn=a.usedIn.map(esc);d.uses={ext:[],int:a.links.map(esc),data:[],vals:[],db:[]};d.loaded=true;
    if(a.blocks){d.blocks=a.blocks.map(blockOf);d.canEdit=!!a.canEdit;d.by=esc(a.owner||'—');d.created=esc(a.created||'');d.place=esc(a.place||'');syncBlocks(d);}
  }catch(e){d.html='<p class="lab">This page could not be loaded.</p>';}
  d.loading=false;render();
}

/* One node of /api/state becomes an object of the mockup's data. */
/* The IDs of the server name files and folders, so they may hold any character. The workspace writes its IDs into its
   markup, so an ID that is not safe there, or that the made-up content uses already, gets a key of its own; SID leads
   from the server's ID to that key. */
const SID={};let keyN=0;
const keyOf=sid=>SID[sid]||(SID[sid]=/[<>"'&`\\]/.test(sid)||sid.length>300||D[sid]?'k'+(++keyN):sid);
const pathOf=sid=>sid.split('/').map(encodeURIComponent).join('/');
function addressOf(key){const sid=D[key].sid||key;return sid.startsWith('f:')?'/f/'+pathOf(sid.slice(2)):'/o/'+pathOf(sid);}
function addNode(node){
  const id=keyOf(node.id),kids=(node.kids||[]).map(addNode),n=esc(node.n);
  if(node.t==='line'||node.t==='space'){decoDoc(id,node.t);D[id].real=true;D[id].sid=node.id;D[id].db=true;return id;}
  if(node.t==='collection'||node.t==='coll'){collDoc(id,n,kids,node.private?'lock':node.owner?'grid':'folder');D[id].real=true;D[id].owner=esc(node.owner||'—');D[id].private=node.private;}
  else D[id]=fill({t:node.t==='folder'?'folder':'page',n,w:'wide',kids,path:esc(node.path||''),by:'—',ch:'',vis:'Everyone signed in',real:true,why:node.t==='folder'&&!node.owner?'A folder of the folder source.':''});
  D[id].sid=node.id;D[id].tags=D[id].tags||[];D[id].db=!!node.owner;D[id].mine=!!node.mine;
  if(node.owner)D[id].owner=esc(node.owner);if(node.access)D[id].acc=node.access;
  return id;
}

function idFromPath(){
  const dec=p=>p.split('/').map(decodeURIComponent).join('/');
  const m=location.pathname.match(/^\/o\/(.+)$/);if(m)return SID[dec(m[1])];
  const f=location.pathname.match(/^\/f\/(.*)$/);if(f)return SID['f:'+dec(f[1]).replace(/\/$/,'')];
  return null;
}

/* Collections of the database: made and renamed through the API (ADR-19). */
const same={credentials:'same-origin',headers:{'Content-Type':'application/json',Accept:'application/json'}};
/* A folder of the database: made in a collection or folder, named in place. */
async function createFolder(parent,title){
  try{const r=await fetch('/api/folders',{...same,method:'POST',body:JSON.stringify({title,parent:D[parent].sid})});if(!r.ok)throw new Error(String(r.status));
    const node=await r.json();node.owner=S.user.name;node.mine=true;const id=addNode(node);D[parent].kids.push(id);S.tog[parent]=true;rebuildPlaces();S.just=id;renameHere(id);}
  catch(e){toast(e.message==='403'?'Others may not add here.':'The folder could not be made.');}
}
/* A collection turns into a folder and back: the same object, shown in nav2 or only in nav3 (ADR-16). */
/* What others may do (ADR-20): only the owner sets it, and the database keeps it; made-up content keeps it here. If the
   server refuses, the level goes back. */
const isOwner=id=>!D[id].db||!!D[id].mine;
async function setAccess(id,level){
  const before=D[id].acc;if(level)D[id].acc=level;else delete D[id].acc;render();
  if(!D[id].db)return;
  try{const r=await fetch('/api/objects/'+encodeURIComponent(D[id].sid)+'/access',{...same,method:'POST',body:JSON.stringify({access:level})});if(!r.ok)throw new Error(String(r.status));}
  catch(e){if(before)D[id].acc=before;else delete D[id].acc;render();toast(e.message==='403'?'Only the owner sets what others may do.':'What others may do could not be saved.');}
}
async function convertKind(id,kind){
  if(D[id].db){try{const r=await fetch('/api/objects/'+encodeURIComponent(D[id].sid)+'/kind',{...same,method:'POST',body:JSON.stringify({kind})});if(!r.ok)throw new Error(String(r.status));}
    catch(e){toast(e.message==='403'?'Others may not change this.':'It could not be turned into a '+kind+'.');return;}}
  D[id].t=kind==='folder'?'folder':'coll';D[id].i=kind==='folder'?'folder':'grid';rebuildPlaces();render();
}
async function createCollection(parent,title){
  try{const r=await fetch('/api/collections',{...same,method:'POST',body:JSON.stringify({title,parent})});if(!r.ok)throw new Error(String(r.status));
    const node=await r.json();node.t='collection';node.owner=S.user.name;node.mine=true;const id=addNode(node);
    if(parent){D[parent].kids.push(id);S.tog[parent]=true;rebuildPlaces();onlyThis(COLL[id]?COLL[id].id:parent);}
    else{C.splice(C.filter(c=>D[c.id]&&D[c.id].real).length,0,asTop({id,i:'grid'}));rebuildPlaces();onlyThis(id);}
    S.view='workspace';S.rail=true;S.just=id;renameHere(id);
  }catch(e){S.toast='The collection could not be made.';render();setTimeout(()=>{S.toast='';render();},2400);}
}
/* The name of something new is written where it appears, in nav2 or in nav3: Enter or leaving the field keeps it,
   Escape keeps the name it got. */
function renameHere(id){S.ren=id;render();const f=document.querySelector('[data-ren]');if(f){f.focus();f.select();}}
function renameDone(x){const id=x.dataset.ren,v=x.value.trim();if(S.ren!==id)return;S.ren=null;
  if(v&&v!==plain(D[id].n)){D[id].n=esc(v);if(D[id].real)renameObject(id,v);}render();}
async function renameObject(id,title){
  try{const r=await fetch('/api/objects/'+encodeURIComponent(id),{...same,method:'PATCH',body:JSON.stringify({title})});if(!r.ok)throw new Error(String(r.status));}
  catch(e){S.toast='The new name could not be saved.';render();setTimeout(()=>{S.toast='';render();},2400);}
}

/* Pages of the database are blocks of Markdown (ADR-19). The view shows each block rendered by the server; Edit turns
   every block into a plain Markdown field, saved when it loses the focus or on Ctrl+Enter, and Escape takes the change
   back. A field keeps what was typed through a render: the draft stays with the block until it is saved. */
const blockOf=b=>({id:esc(b.id),html:b.html,md:esc(b.md),raw:b.md});
function syncBlocks(d){d.doc={intro:d.blocks.map(b=>b.html).join(''),secs:[]};d.ch=d.ch||'';}
const blockIn=x=>{const d=D[x.dataset.d];return d&&d.blocks&&d.blocks.find(b=>b.id===x.dataset.blk);};
function blocksHtml(d,w,i){
  const key=Object.keys(D).find(k=>D[k]===d);
  if(!(w.edit&&d.canEdit))return d.blocks.length?d.blocks.map(b=>`<div id="w${i}${b.id}" data-b="${b.id}">${b.html}</div>`).join(''):`<p class="lab">Empty.${d.canEdit?' Edit adds the first block.':''}</p>`;
  return d.blocks.map(b=>{const text=b.draft!==undefined?esc(b.draft):b.md,rows=Math.min(24,Math.max(2,(b.draft!==undefined?b.draft:b.raw).split('\n').length+1));
    return `<div class="bedit"><textarea data-blk="${b.id}" data-d="${key}" rows="${rows}" spellcheck="true" aria-label="Block ${b.id}, Markdown">${text}</textarea>
      <div class="bbar"><button class="chip" data-act="blkadd" data-w="${i}" data-id="${b.id}">${ic('plus')}Block below</button><button class="chip" data-act="blkdel" data-w="${i}" data-id="${b.id}" aria-pressed="${!!b.ask}">${b.ask?'Delete? Yes':'Delete'}</button><em>${b.saved?'Saved · ':''}#${b.id}</em></div></div>`;}).join('');
}
function refocusBlock(f){if(!f)return;const x=document.querySelector(`textarea[data-blk="${f.id}"][data-d="${f.d}"]`);if(!x)return;x.focus();const p=Math.min(f.pos,x.value.length);x.setSelectionRange(p,p);}
function versionsHtml(d){
  if(!d.versions)return '<div class="lab">Loading…</div>';
  if(!d.versions.length)return '<div class="lab">No changes yet.</div>';
  return `<ul>${d.versions.map(v=>`<li><small>${v.at}</small><span>${v.who} · #${v.block} ${v.deleted?'deleted':v.excerpt||'(empty)'}</span></li>`).join('')}</ul>`;
}
function toast(text){S.toast=text;render();setTimeout(()=>{S.toast='';render();},2600);}
const pageURL=id=>'/api/pages/'+encodeURIComponent(D[id].sid);
async function loadVersions(id){
  try{const r=await fetch(pageURL(id)+'/versions',{credentials:'same-origin',headers:{Accept:'application/json'}});if(!r.ok)throw new Error(String(r.status));
    D[id].versions=(await r.json()).map(v=>({at:esc(v.at),who:esc(v.who),block:esc(v.block),deleted:!!v.deleted,excerpt:esc(v.excerpt)}));}
  catch(e){D[id].versions=[];}
  render();
}
const gearOpen=id=>S.wins.some(w=>w.tabs.some(t=>t.d===id&&t.gear));
async function saveBlock(id,blk,value,field){
  const d=D[id],b=d&&d.blocks&&d.blocks.find(x=>x.id===blk);if(!b)return;
  if(value===b.raw){delete b.draft;return;}
  try{const r=await fetch(pageURL(id)+'/blocks/'+encodeURIComponent(blk),{...same,method:'PUT',body:JSON.stringify({md:value})});if(!r.ok)throw new Error(String(r.status));
    const n=blockOf(await r.json());b.html=n.html;b.md=n.md;b.raw=n.raw;b.saved=true;if(b.draft===value)delete b.draft;syncBlocks(d);d.ch='now';
    /* While the field is still there, only its bar says so, so the focus stays where the writer went; once the page
       left editing, the view shows the new text. */
    const bar=field&&field.isConnected&&field.parentElement.querySelector('.bbar em');if(bar)bar.textContent='Saved · #'+blk;else render();
    if(gearOpen(id))loadVersions(id);}
  catch(e){toast(e.message==='403'?'Others may not change this page.':'This block could not be saved. The text stays in the field.');}
}
async function addBlock(id,after){
  const d=D[id];
  try{const r=await fetch(pageURL(id)+'/blocks',{...same,method:'POST',body:JSON.stringify({after:after||'',md:''})});if(!r.ok)throw new Error(String(r.status));
    const b=blockOf(await r.json()),k=after?d.blocks.findIndex(x=>x.id===after)+1:0;d.blocks.splice(k,0,b);syncBlocks(d);S.focusNext={d:id,id:b.id,pos:0};
    if(gearOpen(id))loadVersions(id);render();}
  catch(e){toast('The block could not be added.');}
}
async function deleteBlock(id,blk){
  const d=D[id],b=d.blocks.find(x=>x.id===blk);if(!b)return;
  /* A block with text asks first, in place: the first click turns the button into the question, the second deletes. */
  if((b.draft!==undefined?b.draft:b.raw).trim()&&!b.ask){d.blocks.forEach(x=>{delete x.ask;});b.ask=true;render();return;}
  try{const r=await fetch(pageURL(id)+'/blocks/'+encodeURIComponent(blk),{...same,method:'DELETE'});if(!r.ok)throw new Error(String(r.status));
    d.blocks=d.blocks.filter(x=>x!==b);syncBlocks(d);if(gearOpen(id))loadVersions(id);render();}
  catch(e){toast('The block could not be deleted.');}
}
/* A new page lands in the collection, folder or page it was made in, and opens with its first block ready to write. */
async function createPage(parent,title){
  try{const r=await fetch('/api/pages',{...same,method:'POST',body:JSON.stringify({title,parent:D[parent].sid})});if(!r.ok)throw new Error(String(r.status));
    const node=await r.json();node.t='page';node.owner=S.user.name;node.mine=true;const id=addNode(node);D[id].canEdit=true;
    D[id].loaded=true;D[id].blocks=[];D[id].by=esc(S.user.name);D[id].created='today';D[id].path='not a file, lives in the database';syncBlocks(D[id]);delete D[id].html;
    D[parent].kids.push(id);S.tog[parent]=true;rebuildPlaces();openDoc(id);S.just=id;
    const w=S.wins[S.focus],t=w&&w.tabs[w.a];if(t&&t.d===id)t.edit=true;
    render();addBlock(id,'');}
  catch(e){toast(e.message==='403'?'Others may not add pages here.':'The page could not be made.');}
}

/* Lines and spaces. In nav2 they are the user's own and stay in this browser; in a collection of the database they are
   objects, shared like everything in it (2026-11-08). */
const decoId=()=>'d'+Date.now().toString(36)+Math.random().toString(36).slice(2,6);
function decoDoc(id,kind){D[id]=fill({t:kind,n:kind==='line'?'Line':'Space',kids:[],path:'',by:'',ch:'',vis:''});return id;}
async function addDivider(kind,cont,after){
  if(!canPlace(cont))return;
  const list=D[cont].kids,k=after?list.indexOf(after)+1:0;
  if(!D[cont].db){list.splice(k,0,decoDoc(decoId(),kind));rebuildPlaces();return;}
  try{const r=await fetch('/api/dividers',{...same,method:'POST',body:JSON.stringify({kind,parent:D[cont].sid,after:after?D[after].sid:''})});if(!r.ok)throw new Error(String(r.status));
    const id=addNode(await r.json());list.splice(after?list.indexOf(after)+1:0,0,id);rebuildPlaces();render();}
  catch(e){toast(e.message==='403'?'Others may not change this collection.':'The '+kind+' could not be added.');}
}
async function removeDivider(id){
  const i=C.findIndex(c=>c.id===id);
  if(i>=0){C.splice(i,1);delete D[id];return;}
  const list=parentList(id),k=list.indexOf(id);
  if(D[id].db){try{const r=await fetch('/api/objects/'+encodeURIComponent(D[id].sid),{...same,method:'DELETE'});if(!r.ok)throw new Error(String(r.status));}
    catch(e){toast(e.message==='403'?'Others may not change this collection.':'The '+D[id].n.toLowerCase()+' could not be removed.');return;}}
  if(k>=0)list.splice(k,1);rebuildPlaces();render();
}
async function moveObject(id,cont,after,undo){
  try{const r=await fetch('/api/objects/'+encodeURIComponent(D[id].sid)+'/move',{...same,method:'POST',body:JSON.stringify({parent:cont?D[cont].sid:'',after:after?D[after].sid:''})});if(!r.ok)throw new Error(String(r.status));return true;}
  catch(e){undo();toast(e.message==='403'?'Others may not change this.':'It could not be moved there.');return false;}
}
/* A copy lands right after what it copies, or where a drop with Ctrl puts it. In the database the server copies the
   whole tree; made-up content is copied here. Only the top copy says it is one. */
function copyLocal(id){const n='k'+(++S.newN),d=D[id];D[n]={...d,kids:d.kids.map(copyLocal)};return n;}
async function copyTo(ids,p,after){
  let prev=after;
  for(const id of ids){
    let n;
    if(D[id].db){try{const r=await fetch('/api/objects/'+encodeURIComponent(D[id].sid)+'/copy',{...same,method:'POST',body:JSON.stringify({parent:p?D[p].sid:'',after:prev?D[prev].sid:''})});if(!r.ok)throw new Error(String(r.status));n=addNode(await r.json());}
      catch(e){toast(e.message==='403'?'Others may not add here.':'It could not be copied.');break;}}
    else{n=copyLocal(id);D[n].n=D[id].n+' (copy)';}
    if(p){const L=D[p].kids;L.splice(prev?L.indexOf(prev)+1:0,0,n);S.tog[p]=true;}
    else C.splice(prev?C.findIndex(c=>c.id===prev)+1:0,0,asTop({id:n,i:D[n].private?'lock':D[n].i||'folder'}));
    prev=n;S.just=n;
  }
  rebuildPlaces();render();
}
/* Deleting takes an entry out of every list, and every window showing it or something below it lets go of it. Its data
   stay, marked, so the made-up content that points at it does not break, and a restore draws it anew. */
function dropLocal(id){
  const gone=[],walk=x=>{gone.push(x);(D[x].kids||[]).forEach(walk);};walk(id);
  const list=parentList(id),k=list.indexOf(id);if(k>=0)list.splice(k,1);
  for(let i=C.length-1;i>=0;i--)if(gone.includes(C[i].id))C.splice(i,1);
  gone.forEach(x=>{D[x].gone=true;delete S.open[x];delete S.pin[x];});
  S.wins.forEach(w=>{w.hist=(w.hist||[]).filter(x=>!gone.includes(x));
    for(let j=w.tabs.length-1;j>=0;j--){if(!gone.includes(w.tabs[j].d))continue;if(w.tabs[j].general)fallback(w);else{w.tabs.splice(j,1);if(w.a>=j&&w.a>0)w.a--;}}});
  compact();S.sel=null;rebuildPlaces();
}
const trashOpen=()=>S.wins.some(w=>w.tabs.some(t=>t.d==='trash:'));
async function deleteObject(id){
  const d=D[id];if(!d)return;
  if(d.db){try{const r=await fetch('/api/objects/'+encodeURIComponent(d.sid),{...same,method:'DELETE'});if(!r.ok)throw new Error(String(r.status));}
    catch(e){toast(e.message==='403'?'Others may not delete this.':'It could not be deleted.');return;}}
  dropLocal(id);toast(d.db?plain(d.n)+' is in the trash.':plain(d.n)+' is deleted.');if(d.db&&trashOpen())loadTrash();
}
/* The trash is a page of the window: what the viewer deleted, and what was theirs, newest first, each with where it lay.
   Restore puts it back there, at the end; deleting for good asks in place as well. */
TYPE.trash=['trash','Trash'];
D['trash:']=fill({t:'trash',n:'Trash',w:'wide',tool:true,path:'not a file, what was deleted',place:'Workspace',by:'',ch:'',vis:'Only you: what you deleted, and what was yours',
  get stat(){return S.trash?S.trash.length+(S.trash.length===1?' entry':' entries'):'Loading…';},
  get html(){const L=S.trash;if(!L)return '<p class="lab">Loading…</p>';if(!L.length)return '<p class="lab">The trash is empty.</p>';
    return `<p class="hint">What you deleted, and what was yours. Restore puts it back where it lay, at the end. What lay in something that is in the trash too comes back after that.</p>
      <div class="tw"><table><tr><th>Name</th><th>Where it lay</th><th>Deleted</th><th>By</th><th></th></tr>${L.map(x=>`<tr data-trash="${x.id}"><td>${x.n}</td><td>${x.place||'Workspace'}${x.placeGone?' (in the trash)':''}</td><td>${x.at}</td><td>${x.who}</td>
        <td><button class="chip" data-act="trrestore" data-id="${x.id}"${x.placeGone?' title="First restore what it lay in"':''}>${ic('back')}Restore</button> <button class="chip" data-act="trpurge" data-id="${x.id}" aria-pressed="${S.trashAsk===x.id}">${ic('trash')}${S.trashAsk===x.id?'Delete for good? Yes':'Delete for good'}</button></td></tr>`).join('')}</table></div>`;}});
async function loadTrash(){
  try{const r=await fetch('/api/trash',{credentials:'same-origin',headers:{Accept:'application/json'}});if(!r.ok)throw new Error(String(r.status));
    S.trash=(await r.json()).map(x=>({id:esc(x.id),n:esc(x.n),t:x.t,place:esc(x.place),placeGone:!!x.placeDeleted,at:esc(x.at),who:esc(x.who)}));}
  catch(e){S.trash=[];toast('The trash could not be read.');return;}
  render();
}
async function restoreTrash(sid){
  const x=(S.trash||[]).find(y=>y.id===sid);
  try{const r=await fetch('/api/trash/'+encodeURIComponent(sid)+'/restore',{...same,method:'POST'});
    if(r.status===400){toast(x&&x.placeGone?'First restore '+plain(x.place.split(' › ').pop())+', where it lay.':'It cannot go back there.');return;}
    if(!r.ok)throw new Error(String(r.status));
    const a=await r.json(),id=addNode(a.node),p=a.parent?SID[a.parent]:'';
    if(a.parent&&!(D[p]&&!D[p].gone)){toast('Restored. Reload to see it where it lay.');loadTrash();return;}
    if(p){D[p].kids.push(id);S.tog[p]=true;}else C.splice(C.filter(c=>D[c.id]&&D[c.id].real).length,0,asTop({id,i:D[id].i}));
    rebuildPlaces();const top=COLL[id]?COLL[id].id:id;if(C.some(c=>c.id===top))S.open[top]=true;S.just=id;
    toast(plain(D[id].n)+' is back.');loadTrash();}
  catch(e){toast(e.message==='403'?'Others may not add there.':'It could not be restored.');}
}
async function purgeTrash(sid){
  try{const r=await fetch('/api/trash/'+encodeURIComponent(sid),{...same,method:'DELETE'});if(!r.ok)throw new Error(String(r.status));toast('Deleted for good.');}
  catch(e){toast('It could not be deleted.');}
  loadTrash();
}
/* Sorting the children by name keeps lines and spaces where they are: each group between them is sorted on its own. */
function sortKids(id,dir){
  const L=D[id].kids,old=L.slice(),groups=[[]];
  L.forEach(x=>{if(isDeco(x))groups.push([x],[]);else groups[groups.length-1].push(x);});
  const by=(a,b)=>dir*plain(D[a].n).localeCompare(plain(D[b].n),undefined,{numeric:true,sensitivity:'base'});
  const out=groups.flatMap(g=>g.length===1&&isDeco(g[0])?g:g.slice().sort(by));
  L.length=0;L.push(...out);rebuildPlaces();render();
  if(D[id].db)moveAll(out.map((x,i)=>[x,id,i?out[i-1]:'']),()=>{L.length=0;L.push(...old);rebuildPlaces();render();});
}
/* Several entries move one after the other, each after the one before, so the server keeps their order. */
async function moveAll(moves,undo){for(const [id,cont,after] of moves)if(!await moveObject(id,cont,after,undo))return;}
/* The order of nav2 with its lines, spaces and shown collections, as the account keeps it. A collection it does not know
   yet keeps its place. */
const n2Of=()=>C.map(c=>isDeco(c.id)?{k:D[c.id].t,id:c.id}:c.alias?{k:'show',id:c.id}:c.id);
function applyN2(list){
  if(!Array.isArray(list))return;const byId={};C.forEach(c=>{byId[c.id]=c;});const out=[];
  list.forEach(x=>{if(x&&typeof x==='object'){if((x.k==='line'||x.k==='space')&&/^d[0-9a-z]{4,20}$/.test(x.id)&&!D[x.id])out.push(asTop({id:decoDoc(x.id,x.k),i:'grid'}));
      else if(x.k==='show'&&typeof x.id==='string'&&D[x.id]&&D[x.id].t==='coll'&&!byId[x.id]&&!out.some(c=>c.id===x.id))out.push(asAlias(x.id));}
    else if(typeof x==='string'&&byId[x]){out.push(byId[x]);delete byId[x];}});
  C.forEach((c,i)=>{if(byId[c.id])out.splice(Math.min(i,out.length),0,c);});
  C.length=0;out.forEach(c=>C.push(c));
}

/* The settings of the workspace: widths, the order of nav2, what is kept open, zoom and theme. */
const KEEP=['s1','s2','w3','fav','pin','limit','scale','theme'];
/* With the database the account keeps them, so they follow it from browser to browser (M-28); a change goes to the
   server a moment later, once, and the browser keeps a copy for the next start and for the workspace without database. */
let sentSettings=null,settingsTimer=0;
function saveSettings(){
  const o={n2:n2Of()};KEEP.forEach(k=>{o[k]=S[k];});const j=JSON.stringify(o);
  try{localStorage.setItem('ud.workspace',j);}catch(e){}
  if(!S.canCreate||j===sentSettings)return;
  clearTimeout(settingsTimer);settingsTimer=setTimeout(()=>{sentSettings=j;fetch('/api/settings',{...same,method:'PUT',body:j}).then(r=>{if(!r.ok)sentSettings=null;}).catch(()=>{sentSettings=null;});},600);
}
/* The account's settings come first; the browser's copy only where the account has none yet. */
function loadSettings(server){
  let o=null;
  if(server&&typeof server==='object'&&!Array.isArray(server)&&Object.keys(server).length)o=server;
  else try{o=JSON.parse(localStorage.getItem('ud.workspace')||'null');}catch(e){o=null;}
  if(!o||typeof o!=='object')return null;
  KEEP.forEach(k=>{if(o[k]!==undefined&&k!=='fav'&&k!=='pin')S[k]=o[k];});return o;
}
async function boot(){
  if(window.C22AppWorkspaceDrag)C22AppWorkspaceDrag.attach(DRAG);
  let st=null;
  /* /dev/ui shows only the made-up content of the mockup: every component in use, nothing from the server. */
  if(!document.getElementById('root').hasAttribute('data-demo'))try{const r=await fetch('/api/state',{credentials:'same-origin',headers:{Accept:'application/json'}});if(r.ok)st=await r.json();}catch(e){}
  if(st){
    S.user=st.user;S.canCreate=st.canCreate;
    const tops=st.collections.map(addNode);
    tops.slice().reverse().forEach(id=>{C.unshift(asTop({id,i:D[id].i}));S.fav[id]=true;});
    if(!st.user.admin){PROFADM.length=0;}
  }
  const kept=loadSettings(st&&st.settings);
  if(kept)applyN2(kept.n2);
  if(kept){Object.keys(kept.fav||{}).forEach(k=>{if(D[k])S.fav[k]=kept.fav[k];});Object.keys(kept.pin||{}).forEach(k=>{if(D[k]){S.pin[k]=kept.pin[k];if(kept.pin[k])S.open[k]=true;}});}
  if(window.udTheme)S.theme=window.udTheme();
  rebuildPlaces();
  const start=idFromPath();
  const first=C.find(c=>D[c.id].real);
  if(start&&D[start]){openDoc(start);const top=COLL[start]?COLL[start].id:start;if(C.some(c=>c.id===top))S.open[top]=true;}
  else if(first){S.open[first.id]=true;const home=D[first.id].kids.find(k=>D[k].t==='page');S.wins=[{tabs:[newTab(home||first.id,true)],a:0,hist:[home||first.id]}];if(home)loadDoc(home);}
  S.booted=true;render();
}
/* A link inside a page to another page opens it in the window, as a click in the list does; Ctrl or the middle button
   leave it to the browser. */
document.addEventListener('click',e=>{const l=e.target.closest&&e.target.closest('article.doc a[href^="/o/"]');if(!l||e.ctrlKey||e.metaKey||e.shiftKey||e.button!==0)return;
  const id=SID[l.getAttribute('href').slice(3).split('#')[0].split('/').map(decodeURIComponent).join('/')];if(!id)return;e.preventDefault();e.stopPropagation();openDoc(id);render();},true);
window.addEventListener('popstate',()=>{const id=idFromPath();if(id&&D[id]){const w=S.wins[S.focus];const k=w.tabs.findIndex(t=>t.d===id);if(k<0){w.tabs[0]=newTab(id,true);w.a=0;}else w.a=k;if(D[id].real&&!D[id].loaded)loadDoc(id);render();}});
boot();
