document.addEventListener("DOMContentLoaded", () => {
    const listContainer = document.getElementById("leaderboard-list");
    const loadingEl = document.getElementById("leaderboard-loading");
    const moreBtn = document.getElementById("leaderboard-more-btn");
    let offset = 0;
    const limit = 20;

    const loadMore = async () => {
        try {
            const response = await fetch(`/api/leaderboard?limit=${limit}&offset=${offset}`);
            if (!response.ok) throw new Error("Failed to load");
            const data = await response.json();
            
            if (data && data.length > 0) {
                data.forEach((entry, index) => {
                    const row = document.createElement("article");
                    row.className = "leaderboard-row";
                    const absoluteIndex = offset + index;
                    
                    row.innerHTML = `
                        <div class="leaderboard-rank">#${absoluteIndex + 1}</div>
                        <div>
                            <p class="leaderboard-name">
                                ${entry.name}${absoluteIndex === 0 ? '<i class="hn hn-crown leaderboard-crown" aria-hidden="true"></i>' : ''}
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
                offset += data.length;
                loadingEl.style.display = "none";
                if (data.length === limit) {
                    moreBtn.style.display = "inline-block";
                } else {
                    moreBtn.style.display = "none";
                }
            } else {
                loadingEl.textContent = offset === 0 ? "No entries yet." : "No more entries.";
                moreBtn.style.display = "none";
            }
        } catch (err) {
            console.error(err);
            loadingEl.textContent = "Error loading leaderboard.";
        }
    };

    moreBtn.addEventListener("click", loadMore);
    loadMore();
});

function formatTime(timestamp) {
    const date = new Date(timestamp * 1000);
    return date.toLocaleString();
}
