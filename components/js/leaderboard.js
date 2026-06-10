document.addEventListener("DOMContentLoaded", () => {
    const listContainer = document.getElementById("leaderboard-list");
    const loadingEl = document.getElementById("leaderboard-loading");
    const moreBtn = document.getElementById("leaderboard-more-btn");
    let offset = 0;
    const limit = 20;
    let isLoading = false;

    let meObserver = new IntersectionObserver((entries) => {
        const myRankBar = document.getElementById("my-rank-bar");
        if (!myRankBar) return;
        entries.forEach(entry => {
            if (entry.isIntersecting) {
                myRankBar.style.transform = "translateY(100%)";
                myRankBar.style.opacity = "0";
                myRankBar.style.pointerEvents = "none";
            } else {
                myRankBar.style.transform = "translateY(0)";
                myRankBar.style.opacity = "1";
                myRankBar.style.pointerEvents = "auto";
            }
        });
    }, { threshold: 0.1 });

    const loadMore = async () => {
        if (isLoading) return;
        isLoading = true;

        const currentOffset = offset;

        try {
            const response = await fetch(`/api/leaderboard?limit=${limit}&offset=${currentOffset}`);
            if (!response.ok) throw new Error("Failed to load");
            const payload = await response.json();

            const data = payload.rows;
            const myRank = payload.my_rank;
            const myEntry = payload.my_entry;

            if (data && data.length > 0) {
                data.forEach((entry, index) => {
                    const absoluteIndex = currentOffset + index;

                    if (document.querySelector(`.leaderboard-row[data-email="${entry.email}"]`)) {
                        return;
                    }

                    const row = document.createElement("article");
                    row.className = "leaderboard-row";
                    row.setAttribute("data-email", entry.email);
                    const isMe = myEntry && entry.email === myEntry.email;

                    if (isMe) {
                        row.classList.add("is-me");
                        meObserver.observe(row);
                    }

                    row.innerHTML = `
                        <div class="leaderboard-rank">${absoluteIndex === 0 ? '<i class="hn hn-crown leaderboard-crown" aria-hidden="true"></i>' : `#${absoluteIndex + 1}`}</div>
                        <div>
                            <p class="leaderboard-name">
                                ${entry.name}
                            </p>
                            <p class="leaderboard-email mono">${entry.email}</p>
                        </div>
                        <div class="leaderboard-score">${entry.level}</div>
                        <div class="leaderboard-time">${formatTime(entry.time)}</div>
                    `;
                    if (listContainer.contains(moreBtn.parentNode)) {
                        listContainer.insertBefore(row, moreBtn.parentNode);
                    } else {
                        listContainer.appendChild(row);
                    }
                });

                if (myEntry && myRank && currentOffset === 0) {
                    const myRankBar = document.getElementById("my-rank-bar");
                    const myRankRow = document.getElementById("my-rank-row");

                    myRankBar.classList.remove("hidden");
                    myRankRow.innerHTML = `
                        <div class="leaderboard-rank">${myRank === 1 ? '<i class="hn hn-crown leaderboard-crown" aria-hidden="true"></i>' : `#${myRank}`}</div>
                        <div>
                            <p class="leaderboard-name">${myEntry.name} (You)</p>
                            <p class="leaderboard-email mono">${myEntry.email}</p>
                        </div>
                        <div class="leaderboard-score">${myEntry.level}</div>
                    `;

                    const newMyRankRow = myRankRow.cloneNode(true);
                    myRankRow.parentNode.replaceChild(newMyRankRow, myRankRow);

                    newMyRankRow.addEventListener("click", async () => {
                        let meRow = document.querySelector(".leaderboard-row.is-me");
                        while (!meRow && moreBtn.style.display !== "none") {
                            await loadMore();
                            meRow = document.querySelector(".leaderboard-row.is-me");
                            if (!meRow) await new Promise(r => setTimeout(r, 100));
                        }
                        if (meRow) {
                            meRow.scrollIntoView({ behavior: "smooth", block: "center" });
                            meRow.style.animation = "none";
                            meRow.offsetHeight;
                            meRow.style.animation = "rowHighlight 2s ease";
                        }
                    });
                }

                offset += data.length;
                loadingEl.style.display = "none";
                if (data.length === limit) {
                    moreBtn.style.display = "inline-block";
                } else {
                    moreBtn.style.display = "none";
                }
            } else {
                loadingEl.innerHTML = offset === 0 ? "No entries yet." : "No more entries.";
                loadingEl.className = "empty-state";
                loadingEl.style.display = "block";
                moreBtn.style.display = "none";
            }
        } catch (err) {
            console.error(err);
            loadingEl.innerHTML = "Error loading leaderboard.";
            loadingEl.className = "empty-state";
            loadingEl.style.display = "block";
        } finally {
            isLoading = false;
        }
    };

    moreBtn.addEventListener("click", loadMore);
    loadMore();
});

function formatTime(timestamp) {
    const date = new Date(timestamp * 1000);
    return date.toLocaleString();
}
